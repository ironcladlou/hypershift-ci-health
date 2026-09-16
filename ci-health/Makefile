APP := ci-health
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)
NAMESPACE := ci-health
ROUTE_PREFIX := hypershift-ci-health

.PHONY: build fmt test generate-job-registry update-golden image setup deploy

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) .

fmt:
	gofmt -w $$(rg --files --glob '*.go')

test:
	go test ./...

generate-job-registry: build
	@test -n "$(RELEASE_DIR)" || (echo "RELEASE_DIR is required" >&2; exit 1)
	./$(BIN) job-registry --release-dir=$(RELEASE_DIR) > job-registry.json

update-golden: build
	@test -n "$(RELEASE_DIR)" || (echo "RELEASE_DIR is required" >&2; exit 1)
	./$(BIN) job-registry --release-dir=$(RELEASE_DIR) > jobregistry/testdata/job-registry.json

image:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN).linux .
	mv $(BIN) $(BIN).native 2>/dev/null || true
	mv $(BIN).linux $(BIN)
	oc -n $(NAMESPACE) start-build $(APP) --from-dir=. --follow; \
		s=$$?; mv $(BIN).native $(BIN) 2>/dev/null || rm -f $(BIN); exit $$s

setup:
	@DOMAIN=$$(oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}') && \
	printf 'apiVersion: route.openshift.io/v1\nkind: Route\nmetadata:\n  name: ci-health\nspec:\n  host: $(ROUTE_PREFIX).%s\n' "$$DOMAIN" > deploy/route-patch.yaml && \
	echo "Wrote deploy/route-patch.yaml ($(ROUTE_PREFIX).$$DOMAIN)"

deploy:
	oc apply -k deploy/
	$(MAKE) image
	oc -n $(NAMESPACE) rollout restart deployment/$(APP)
	oc -n $(NAMESPACE) rollout status deployment/$(APP) --timeout=120s
	@oc -n $(NAMESPACE) get route $(APP) -o jsonpath='https://{.spec.host}{"\n"}'
