# Include toolbox tasks
include ./.toolbox.mk

lint: tb.golangci-lint
	$(TB_GOLANGCI_LINT) run --fix

tidy:
	go mod tidy

test: tidy lint
	go test ./...  -coverprofile=coverage.out
	go tool cover -func=coverage.out

vhs-pod:
	kubectl apply -f docs/PodMemoryLoadDemo.yaml
	kubectl wait --for=condition=ready pod two-container-memory-demo -n default --timeout=60s
	@for i in $$(seq 1 20); do \
		kubectl get podmetrics.metrics.k8s.io -n default two-container-memory-demo >/dev/null 2>&1 && exit 0; \
		sleep 1; \
	done; \
	kubectl get podmetrics.metrics.k8s.io -n default two-container-memory-demo
	vhs docs/pod.tape
	kubectl delete -f docs/PodMemoryLoadDemo.yaml --force

vhs-node:
	kubectl apply -f docs/PodMemoryLoadDemo.yaml
	kubectl wait --for=condition=ready pod two-container-memory-demo -n default --timeout=60s
	vhs docs/node.tape
	kubectl delete -f docs/PodMemoryLoadDemo.yaml --force

release: tb.semver tb.goreleaser tb.syft
	@version=$$($(TB_LOCALBIN)/semver); \
	git tag -s $$version -m"Release $$version"
	PATH=$(TB_LOCALBIN):$${PATH} $(TB_GORELEASER) --clean --parallelism 2

test-release: tb.goreleaser tb.syft
	PATH=$(TB_LOCALBIN):$${PATH} $(TB_GORELEASER) --skip=publish --snapshot --clean --parallelism 2

