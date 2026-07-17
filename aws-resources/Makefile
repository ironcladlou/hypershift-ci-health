BIN := aws-resources
NAMESPACE := ci-health-aws-resources

.PHONY: build build-linux image setup teardown deploy

build:
	go build -o $(BIN) .

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN) .

image: build-linux
	oc -n $(NAMESPACE) start-build $(BIN) --from-dir=. --follow

setup: build
	./$(BIN) setup

teardown: build
	./$(BIN) teardown

deploy:
	oc apply -k deploy/
	$(MAKE) image
	oc -n $(NAMESPACE) rollout restart deployment/$(BIN)
	oc -n $(NAMESPACE) rollout status deployment/$(BIN) --timeout=60s
	@oc -n $(NAMESPACE) get route $(BIN) -o jsonpath='https://{.spec.host}{"\n"}'
