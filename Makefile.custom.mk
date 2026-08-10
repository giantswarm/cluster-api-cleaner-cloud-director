##@ Integration tests

# Integration tests run locally only. They are guarded by the `integration` build
# tag, so `go test ./...` and CI do not compile them.

ENVTEST_K8S_VERSION ?= 1.36.2
ENVTEST_VERSION     ?= v0.24.1
ENVTEST_ASSETS_DIR  ?= $(shell pwd)/bin/envtest
ENVTEST             ?= go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)

.PHONY: integration-test
integration-test: ## Runs the local integration tests (envtest + fake VCD api).
	@echo "====> $@"
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(ENVTEST_ASSETS_DIR) -p path)" \
		go test -tags=integration $(RACE) -count=1 -timeout=10m ./test/integration/...
