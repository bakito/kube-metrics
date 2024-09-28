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

release: tb.semver tb.goreleaser
	@version=$$($(TB_LOCALBIN)/semver); \
	git tag -s $$version -m"Release $$version"
	$(TB_GORELEASER) --clean

test-release: tb.goreleaser
	$(TB_GORELEASER)  --skip=publish --snapshot --clean

