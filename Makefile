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
	vhs docs/pod.tape

vhs-node:
	vhs docs/node.tape

release: tb.semver tb.goreleaser tb.syft
	@version=$$($(TB_LOCALBIN)/semver); \
	git tag -s $$version -m"Release $$version"
	PATH=$(TB_LOCALBIN):$${PATH} $(TB_GORELEASER) --clean --parallelism 2

test-release: tb.goreleaser tb.syft
	PATH=$(TB_LOCALBIN):$${PATH} $(TB_GORELEASER) --skip=publish --snapshot --clean --parallelism 2

