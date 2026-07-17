package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"
)

const (
	roleName   = "hypershift-ci-aws-resources"
	rolePath   = "/hypershift-ci/"
	policyName = "orphan-resource-discovery"

	tagManagedBy = "managed-by"
	tagComponent = "component"
	tagPurpose   = "purpose"

	tagValueManagedBy = "hypershift-ci-health"
	tagValueComponent = "aws-resources"
	tagValuePurpose   = "ci-orphan-detection"

	saPatchFileRel    = "aws-resources/deploy/sa-role-patch.yaml"
	routePatchFileRel = "aws-resources/deploy/route-patch.yaml"
	routeHostPrefix   = "hypershift-ci-aws-resources"
)

var managedTags = []iamtypes.Tag{
	{Key: aws.String(tagManagedBy), Value: aws.String(tagValueManagedBy)},
	{Key: aws.String(tagComponent), Value: aws.String(tagValueComponent)},
	{Key: aws.String(tagPurpose), Value: aws.String(tagValuePurpose)},
}

func setupCmd() *cobra.Command {
	var (
		oidcProviderARN string
		namespace       string
		serviceAccount  string
		dryRun          bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Provision IAM role and cluster-specific Kustomize patches",
		Long: `Idempotently provisions everything needed for in-cluster deployment:

IAM: Creates (or updates) the IAM role and inline policy for IRSA-based
AWS access. The OIDC provider is auto-discovered from the current
kubeconfig's cluster unless --oidc-provider-arn is set. The AWS account
ID is auto-detected via sts:GetCallerIdentity. Resources use deterministic
names and are tagged for positive identification:
  Role:   hypershift-ci-aws-resources  (path /hypershift-ci/)
  Policy: orphan-resource-discovery    (inline on the role)

Route: Discovers the cluster's ingress domain and writes a Kustomize
patch with the route hostname.

Both patches are gitignored. This command is safe to re-run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(cmd.Context(), oidcProviderARN, namespace, serviceAccount, dryRun)
		},
	}

	cmd.Flags().StringVar(&oidcProviderARN, "oidc-provider-arn", "", "OIDC provider ARN (auto-discovered from cluster if omitted)")
	cmd.Flags().StringVar(&namespace, "namespace", "ci-health-aws-resources", "Kubernetes namespace for the trust policy")
	cmd.Flags().StringVar(&serviceAccount, "service-account", "aws-resources", "Kubernetes ServiceAccount name for the trust policy")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be done without making changes")

	return cmd
}

func teardownCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove IAM role and generated patch files",
		Long: `Removes the IAM role, inline policy, and generated Kustomize patch
files created by the setup command. Refuses to delete the IAM role if its
tags don't match (prevents accidental deletion of unrelated roles).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTeardown(cmd.Context(), dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be done without making changes")

	return cmd
}

func runSetup(ctx context.Context, oidcProviderARN, namespace, serviceAccount string, dryRun bool) error {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(awsCfg)
	iamClient := iam.NewFromConfig(awsCfg)

	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("getting caller identity: %w", err)
	}
	accountID := aws.ToString(identity.Account)
	fmt.Fprintf(os.Stderr, "AWS account: %s\n", accountID)

	if oidcProviderARN == "" {
		oidcProviderARN, err = discoverOIDCProvider(ctx, iamClient, accountID)
		if err != nil {
			return fmt.Errorf("auto-discovering OIDC provider: %w\n\nSet --oidc-provider-arn explicitly if the cluster is not reachable via kubeconfig", err)
		}
	}

	oidcIssuer, err := oidcIssuerFromARN(oidcProviderARN)
	if err != nil {
		return err
	}

	roleARN := fmt.Sprintf("arn:aws:iam::%s:role%s%s", accountID, rolePath, roleName)

	fmt.Fprintf(os.Stderr, "OIDC provider: %s\n", oidcProviderARN)
	fmt.Fprintf(os.Stderr, "OIDC issuer:   %s\n", oidcIssuer)
	fmt.Fprintf(os.Stderr, "Role ARN:      %s\n", roleARN)
	fmt.Fprintf(os.Stderr, "Trust:         system:serviceaccount:%s:%s\n", namespace, serviceAccount)
	fmt.Fprintln(os.Stderr)

	ingressDomain, err := discoverIngressDomain(ctx)
	if err != nil {
		return fmt.Errorf("discovering ingress domain: %w", err)
	}
	routeHost := routeHostPrefix + "." + ingressDomain
	fmt.Fprintf(os.Stderr, "Route host:    %s\n", routeHost)

	if dryRun {
		fmt.Fprintln(os.Stderr, "[dry-run] Would create/update IAM role and inline policy")
		fmt.Fprintln(os.Stderr, "[dry-run] Would write Kustomize patches to", saPatchFileRel, "and", routePatchFileRel)
		return nil
	}

	trustPolicy, err := buildTrustPolicy(oidcProviderARN, oidcIssuer, namespace, serviceAccount)
	if err != nil {
		return fmt.Errorf("building trust policy: %w", err)
	}

	permissionsPolicy, err := buildPermissionsPolicy()
	if err != nil {
		return fmt.Errorf("building permissions policy: %w", err)
	}

	if err := ensureRole(ctx, iamClient, trustPolicy); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Putting inline policy %q...\n", policyName)
	if _, err := iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(permissionsPolicy),
	}); err != nil {
		return fmt.Errorf("putting role policy: %w", err)
	}

	if err := writeSAPatch(roleARN); err != nil {
		return fmt.Errorf("writing SA patch: %w", err)
	}

	if err := writeRoutePatch("aws-resources", routeHost, routePatchFileRel); err != nil {
		return fmt.Errorf("writing route patch: %w", err)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Setup complete. Next steps:")
	fmt.Fprintf(os.Stderr, "  1. Deploy: make deploy\n")
	fmt.Fprintf(os.Stderr, "  2. Verify: oc -n %s get sa %s -o yaml\n", namespace, serviceAccount)

	return nil
}

func runTeardown(ctx context.Context, dryRun bool) error {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	iamClient := iam.NewFromConfig(awsCfg)

	getRoleOut, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		var notFound *iamtypes.NoSuchEntityException
		if errors.As(err, &notFound) {
			fmt.Fprintln(os.Stderr, "Role does not exist, nothing to tear down.")
			return nil
		}
		return fmt.Errorf("getting role: %w", err)
	}

	if !hasExpectedTags(getRoleOut.Role.Tags) {
		return fmt.Errorf("role %q exists but is not tagged as managed by %s — refusing to delete", roleName, tagValueManagedBy)
	}

	fmt.Fprintf(os.Stderr, "Role:   %s\n", aws.ToString(getRoleOut.Role.Arn))
	fmt.Fprintf(os.Stderr, "Policy: %s\n", policyName)
	fmt.Fprintln(os.Stderr)

	if dryRun {
		fmt.Fprintln(os.Stderr, "[dry-run] Would delete inline policy and IAM role")
		fmt.Fprintln(os.Stderr, "[dry-run] Would remove", saPatchFileRel, "and", routePatchFileRel)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Deleting inline policy %q...\n", policyName)
	if _, err := iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(roleName),
		PolicyName: aws.String(policyName),
	}); err != nil {
		var notFound *iamtypes.NoSuchEntityException
		if !errors.As(err, &notFound) {
			return fmt.Errorf("deleting role policy: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Deleting role %q...\n", roleName)
	if _, err := iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	}); err != nil {
		return fmt.Errorf("deleting role: %w", err)
	}

	for _, rel := range []string{saPatchFileRel, routePatchFileRel} {
		path, err := repoFile(rel)
		if err != nil {
			return fmt.Errorf("resolving path for %s: %w", rel, err)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", rel, err)
		} else if err == nil {
			fmt.Fprintf(os.Stderr, "Removed %s\n", rel)
		}
	}

	fmt.Fprintln(os.Stderr, "Teardown complete.")
	return nil
}

func discoverOIDCProvider(ctx context.Context, iamClient *iam.Client, accountID string) (string, error) {
	out, err := exec.CommandContext(ctx, "oc", "get", "authentication", "cluster",
		"-o", "jsonpath={.spec.serviceAccountIssuer}").Output()
	if err != nil {
		return "", fmt.Errorf("running oc get authentication: %w (is your kubeconfig set?)", err)
	}

	issuerURL := strings.TrimSpace(string(out))
	if issuerURL == "" {
		return "", fmt.Errorf("cluster returned empty serviceAccountIssuer")
	}

	parsed, err := url.Parse(issuerURL)
	if err != nil {
		return "", fmt.Errorf("parsing issuer URL %q: %w", issuerURL, err)
	}
	provider := parsed.Host + parsed.Path

	providerARN := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, provider)

	// Verify the provider exists in IAM
	if _, err := iamClient.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerARN),
	}); err != nil {
		return "", fmt.Errorf("OIDC provider %s not found in IAM: %w", providerARN, err)
	}

	fmt.Fprintf(os.Stderr, "Auto-discovered OIDC provider from cluster\n")
	return providerARN, nil
}

func oidcIssuerFromARN(arn string) (string, error) {
	const prefix = "oidc-provider/"
	idx := strings.Index(arn, prefix)
	if idx < 0 {
		return "", fmt.Errorf("cannot extract issuer from OIDC provider ARN %q (expected 'oidc-provider/' segment)", arn)
	}
	return arn[idx+len(prefix):], nil
}

func ensureRole(ctx context.Context, iamClient *iam.Client, trustPolicy string) error {
	getRoleOut, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		var notFound *iamtypes.NoSuchEntityException
		if !errors.As(err, &notFound) {
			return fmt.Errorf("checking for existing role: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Creating role %q...\n", roleName)
		if _, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			Path:                     aws.String(rolePath),
			AssumeRolePolicyDocument: aws.String(trustPolicy),
			Description:              aws.String("IRSA role for HyperShift CI orphaned resource discovery"),
			Tags:                     managedTags,
		}); err != nil {
			return fmt.Errorf("creating role: %w", err)
		}
		return nil
	}

	if !hasExpectedTags(getRoleOut.Role.Tags) {
		return fmt.Errorf("role %q exists but is not tagged as managed by %s — refusing to modify", roleName, tagValueManagedBy)
	}

	fmt.Fprintf(os.Stderr, "Role %q exists, updating trust policy...\n", roleName)
	if _, err := iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyDocument: aws.String(trustPolicy),
	}); err != nil {
		return fmt.Errorf("updating assume role policy: %w", err)
	}

	if _, err := iamClient.TagRole(ctx, &iam.TagRoleInput{
		RoleName: aws.String(roleName),
		Tags:     managedTags,
	}); err != nil {
		return fmt.Errorf("tagging role: %w", err)
	}

	return nil
}

func hasExpectedTags(tags []iamtypes.Tag) bool {
	found := map[string]string{}
	for _, t := range tags {
		found[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return found[tagManagedBy] == tagValueManagedBy
}

type policyDocument struct {
	Version   string            `json:"Version"`
	Statement []policyStatement `json:"Statement"`
}

type policyStatement struct {
	Effect    string            `json:"Effect"`
	Action    any               `json:"Action"`
	Resource  any               `json:"Resource,omitempty"`
	Principal map[string]string `json:"Principal,omitempty"`
	Condition map[string]any    `json:"Condition,omitempty"`
}

func buildTrustPolicy(oidcProviderARN, oidcIssuer, namespace, serviceAccount string) (string, error) {
	doc := policyDocument{
		Version: "2012-10-17",
		Statement: []policyStatement{{
			Effect: "Allow",
			Action: "sts:AssumeRoleWithWebIdentity",
			Principal: map[string]string{
				"Federated": oidcProviderARN,
			},
			Condition: map[string]any{
				"StringEquals": map[string]string{
					oidcIssuer + ":sub": fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccount),
				},
			},
		}},
	}
	return marshalPolicy(doc)
}

func buildPermissionsPolicy() (string, error) {
	doc := policyDocument{
		Version: "2012-10-17",
		Statement: []policyStatement{{
			Effect:   "Allow",
			Action:   "tag:GetResources",
			Resource: "*",
		}},
	}
	return marshalPolicy(doc)
}

func marshalPolicy(doc policyDocument) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func repoFile(rel string) (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("finding repo root: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), rel), nil
}

func writePatch(rel, content string) error {
	path, err := repoFile(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", rel)
	return nil
}

func writeSAPatch(roleARN string) error {
	return writePatch(saPatchFileRel, fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: aws-resources
  annotations:
    eks.amazonaws.com/role-arn: %s
`, roleARN))
}

func writeRoutePatch(routeName, host, rel string) error {
	return writePatch(rel, fmt.Sprintf(`apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: %s
spec:
  host: %s
`, routeName, host))
}

func discoverIngressDomain(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "oc", "get", "ingresses.config.openshift.io", "cluster",
		"-o", "jsonpath={.spec.domain}").Output()
	if err != nil {
		return "", fmt.Errorf("running oc get ingresses.config: %w", err)
	}
	domain := strings.TrimSpace(string(out))
	if domain == "" {
		return "", fmt.Errorf("cluster returned empty ingress domain")
	}
	return domain, nil
}
