lint: golangci-lint
	$(GOLANGCI_LINT) run --fix

tidy:
	go mod tidy

test: tidy lint
	go test ./...  -coverprofile=coverage.out
	go tool cover -func=coverage.out

vhs-pod:
	vhs docs/pod.tape

vhs-node:
	vhs docs/node.tape

release: semver goreleaser
	@version=$$($(LOCALBIN)/semver); \
	git tag -s $$version -m"Release $$version"
	$(GORELEASER) --clean


test-release: goreleaser
	$(GORELEASER)  --skip-publish --snapshot --clean

## toolbox - start
## Current working directory
LOCALDIR ?= $(shell which cygpath > /dev/null 2>&1 && cygpath -m $$(pwd) || pwd)
## Location to install dependencies to
LOCALBIN ?= $(LOCALDIR)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
SEMVER ?= $(LOCALBIN)/semver
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
GORELEASER ?= $(LOCALBIN)/goreleaser

## Tool Versions
SEMVER_VERSION ?= v1.1.3
GOLANGCI_LINT_VERSION ?= v1.52.2
GORELEASER_VERSION ?= v1.18.2

## Tool Installer
.PHONY: semver
semver: $(SEMVER) ## Download semver locally if necessary.
$(SEMVER): $(LOCALBIN)
	test -s $(LOCALBIN)/semver || GOBIN=$(LOCALBIN) go install github.com/bakito/semver@$(SEMVER_VERSION)
.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint || GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
.PHONY: goreleaser
goreleaser: $(GORELEASER) ## Download goreleaser locally if necessary.
$(GORELEASER): $(LOCALBIN)
	test -s $(LOCALBIN)/goreleaser || GOBIN=$(LOCALBIN) go install github.com/goreleaser/goreleaser@$(GORELEASER_VERSION)

## Update Tools
.PHONY: update-toolbox-tools
update-toolbox-tools:
	@rm -f \
		$(LOCALBIN)/semver \
		$(LOCALBIN)/golangci-lint \
		$(LOCALBIN)/goreleaser
	toolbox makefile -f $(LOCALDIR)/Makefile \
		github.com/bakito/semver \
		github.com/golangci/golangci-lint/cmd/golangci-lint \
		github.com/goreleaser/goreleaser
## toolbox - end
