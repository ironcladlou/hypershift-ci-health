BIN := ci-health
NAMESPACE := ci-health
ROUTE_PREFIX := hypershift-ci-health

.PHONY: build image setup deploy

build:
	go build -o $(BIN) .

image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN).linux .
	mv $(BIN) $(BIN).native 2>/dev/null || true
	mv $(BIN).linux $(BIN)
	oc -n $(NAMESPACE) start-build $(BIN) --from-dir=. --follow; \
		s=$$?; mv $(BIN).native $(BIN) 2>/dev/null || rm -f $(BIN); exit $$s

setup:
	@if [ -z "$(TOKEN_FILE)" ]; then \
		echo "Usage: make setup TOKEN_FILE=/path/to/github-pat.txt"; \
		exit 1; \
	fi
	@cp $(TOKEN_FILE) deploy/.github-token
	@echo "Wrote deploy/.github-token"
	@DOMAIN=$$(oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}') && \
	printf 'apiVersion: route.openshift.io/v1\nkind: Route\nmetadata:\n  name: ci-health\nspec:\n  host: $(ROUTE_PREFIX).%s\n' "$$DOMAIN" > deploy/route-patch.yaml && \
	echo "Wrote deploy/route-patch.yaml ($(ROUTE_PREFIX).$$DOMAIN)"

deploy:
	oc apply -k deploy/
	$(MAKE) image
	oc -n $(NAMESPACE) rollout restart deployment/$(BIN)
	oc -n $(NAMESPACE) rollout status deployment/$(BIN) --timeout=120s
	@oc -n $(NAMESPACE) get route $(BIN) -o jsonpath='https://{.spec.host}{"\n"}'
