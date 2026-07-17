NAMESPACE := ci-health
ROUTE_PREFIX := hypershift-ci-health

.PHONY: setup deploy

setup:
	@DOMAIN=$$(oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}') && \
	printf 'apiVersion: route.openshift.io/v1\nkind: Route\nmetadata:\n  name: ci-health\nspec:\n  host: $(ROUTE_PREFIX).%s\n' "$$DOMAIN" > deploy/route-patch.yaml && \
	echo "Wrote deploy/route-patch.yaml ($(ROUTE_PREFIX).$$DOMAIN)"

deploy:
	oc apply -k deploy/
	oc -n $(NAMESPACE) create configmap ci-health-html \
		--from-file=index.html=index.html \
		--dry-run=client -o yaml | oc apply -f -
	oc -n $(NAMESPACE) rollout restart deployment/ci-health
	oc -n $(NAMESPACE) rollout status deployment/ci-health --timeout=60s
	@oc -n $(NAMESPACE) get route ci-health -o jsonpath='https://{.spec.host}{"\n"}'
