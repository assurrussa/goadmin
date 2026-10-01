GO_SHARED_CACHE_ROOT ?= $(HOME)/dev/projects/.cache/go
GOCACHE ?= $(GO_SHARED_CACHE_ROOT)/build
GOMODCACHE ?= $(GO_SHARED_CACHE_ROOT)/mod
GOLANGCI_LINT_CACHE ?= $(GO_SHARED_CACHE_ROOT)/lint
GOPATH ?= $(shell GOTOOLCHAIN=local go env GOPATH)
export GOCACHE GOMODCACHE GOLANGCI_LINT_CACHE

.DEFAULT_GOAL := full

.PHONY: full prepare check tidy-check tidy generate fmt fmt-check lint lint-fix vet test test-full test-race integration-check security-negative-check govulncheck release-security-check bench-all cover-html resources resources-audit node-version-check externalconsumer-local externalconsumer-published publish-readiness release-readiness release-version-check import-policy-site

GO_MODULE := $(shell GOWORK=off go list -m)
VERSION ?=
GOAUTH_LOCAL_PATH ?=
GOUPLOADS_LOCAL_PATH ?=
GONOTIFY_LOCAL_PATH ?=
GOWEBSOCKET_LOCAL_PATH ?=
export GOAUTH_LOCAL_PATH GONOTIFY_LOCAL_PATH GOUPLOADS_LOCAL_PATH GOWEBSOCKET_LOCAL_PATH
SITE_REPO ?= ../site
PROJECT_GOCACHE ?= $(GOCACHE)
PROJECT_GOMODCACHE ?= $(GOMODCACHE)
PROJECT_GOPATH ?= $(GOPATH)
PROJECT_NPM_CACHE ?= $(CURDIR)/.go-cache/npm
PROJECT_GOLANGCI_LINT_CACHE ?= $(GOLANGCI_LINT_CACHE)
GOVULNCHECK ?= govulncheck
GO_FILES := $(shell find . -type f -name '*.go' -not -path './.cache/*' -not -path './.go-cache/*' -not -path './tmp/*' -not -path './vendor/*' -not -path './resources/node_modules/*')
GO_PACKAGES := ./...
export GOCACHE := $(PROJECT_GOCACHE)
export GOMODCACHE := $(PROJECT_GOMODCACHE)
export GOPATH := $(PROJECT_GOPATH)
export npm_config_cache := $(PROJECT_NPM_CACHE)
export GOLANGCI_LINT_CACHE := $(PROJECT_GOLANGCI_LINT_CACHE)

full: prepare
	$(MAKE) check

prepare:
	$(MAKE) tidy
	$(MAKE) generate
	$(MAKE) fmt
	$(MAKE) lint-fix

check: tidy-check fmt-check vet lint test-full resources

release-version-check:
	@printf '%s\n' "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$$' || \
		(echo "VERSION must be an exact semver tag" && exit 2)

publish-readiness: release-version-check check integration-check release-security-check externalconsumer-public-deps-local
	@git diff --exit-code

release-readiness: release-version-check check integration-check release-security-check externalconsumer-published

release-security-check: security-negative-check govulncheck resources-audit

tidy-check:
	go mod tidy -diff

tidy:
	go mod tidy

generate:
	go generate $(GO_PACKAGES)

fmt:
	go fmt $(GO_PACKAGES)
	gofumpt -l -w $(GO_FILES)
	gci write -s standard -s default -s "prefix($(GO_MODULE))" $(GO_FILES)

fmt-check:
	@unformatted="$$(gofumpt -l $(GO_FILES))" || exit $$?; \
		test -z "$$unformatted" || { printf 'gofumpt changes are required:\n%s\nRun: make prepare\n' "$$unformatted" >&2; exit 1; }
	@import_diff="$$(gci diff -s standard -s default -s "prefix($(GO_MODULE))" $(GO_FILES))" || exit $$?; \
		test -z "$$import_diff" || { printf 'gci changes are required:\n%s\nRun: make prepare\n' "$$import_diff" >&2; exit 1; }

lint:
	golangci-lint run -v --timeout=5m ./...

lint-fix:
	golangci-lint run -v --fix --timeout=5m ./...

vet:
	go vet $(GO_PACKAGES)

test:
	go test $(GO_PACKAGES)

test-full:
	go test -race -cover -covermode=atomic -count=1 $(GO_PACKAGES)

integration-check:
	GOWORK=off go test -tags=integration -count=1 $(GO_PACKAGES)

security-negative-check:
	@set -eu; tests="$$(go test ./host -list '^(TestValidateSetupSecretOutputRejectsStandardStreamsAndLooseFiles|TestIssueFirstAdminSetupTokenWritesSecretOnlyToSecretChannel|TestIssueFirstAdminSetupTokenRevokesSecretWhenOutputFails|TestValidateFirstAdminSetupURLRejectsCredentialAndTokenLeakage)$$')"; \
	for test in TestValidateSetupSecretOutputRejectsStandardStreamsAndLooseFiles TestIssueFirstAdminSetupTokenWritesSecretOnlyToSecretChannel TestIssueFirstAdminSetupTokenRevokesSecretWhenOutputFails TestValidateFirstAdminSetupURLRejectsCredentialAndTokenLeakage; do \
		echo "$$tests" | grep -Fx "$$test" >/dev/null || { echo "required security test missing: host/$$test" >&2; exit 1; }; \
	done
	go test ./host -run '^(TestValidateSetupSecretOutputRejectsStandardStreamsAndLooseFiles|TestIssueFirstAdminSetupTokenWritesSecretOnlyToSecretChannel|TestIssueFirstAdminSetupTokenRevokesSecretWhenOutputFails|TestValidateFirstAdminSetupURLRejectsCredentialAndTokenLeakage)$$' -count=1
	@set -eu; tests="$$(go test ./internal/setupcmd -list '^(TestRunNonInteractiveWritesRawTokenOnlyToSecretFile|TestRunNonInteractiveWithoutSecretChannelStopsBeforeIssue|TestOpenSecretFileRejectsSymlink)$$')"; \
	for test in TestRunNonInteractiveWritesRawTokenOnlyToSecretFile TestRunNonInteractiveWithoutSecretChannelStopsBeforeIssue TestOpenSecretFileRejectsSymlink; do \
		echo "$$tests" | grep -Fx "$$test" >/dev/null || { echo "required security test missing: internal/setupcmd/$$test" >&2; exit 1; }; \
	done
	go test ./internal/setupcmd -run '^(TestRunNonInteractiveWritesRawTokenOnlyToSecretFile|TestRunNonInteractiveWithoutSecretChannelStopsBeforeIssue|TestOpenSecretFileRejectsSymlink)$$' -count=1

govulncheck:
	@command -v "$(GOVULNCHECK)" >/dev/null 2>&1 || { echo "$(GOVULNCHECK) is required for release readiness; install golang.org/x/vuln/cmd/govulncheck and retry" >&2; exit 1; }
	$(GOVULNCHECK) ./...

test-race:
	go test -race -count=5 $(GO_PACKAGES)

bench-all:
	go test -bench=. -benchmem $(GO_PACKAGES)

cover-html:
	@packages="$(GO_PACKAGES)"; \
	go test -coverprofile=./coverage.text -covermode=atomic $$packages; \
	go tool cover -html=./coverage.text -o ./cover.html; \
	rm ./coverage.text

node-version-check:
	@node -e 'const [major, minor] = process.versions.node.split(".").map(Number); if (!((major === 22 && minor >= 13) || major >= 24)) { console.error(`Node.js 22.13+ on the 22.x line or >=24 is required, got $${process.versions.node}`); process.exit(2) }'

resources: node-version-check
	cd resources && npm ci && npm run test:manifest && npm run test:unit && npm exec -- eslint . && npm exec -- prettier --check src/ && npm run type-check && npm run build-only

resources-audit: node-version-check
	cd resources && npm audit --audit-level=high

externalconsumer-local:
	@GOWORK=off GOFLAGS= bash scripts/anonymous-consumer.sh shared-local

externalconsumer-candidates:
	@GOWORK=off GOFLAGS= bash scripts/anonymous-consumer.sh shared-candidates

externalconsumer-published:
	GOWORK=off GOFLAGS= bash scripts/anonymous-consumer.sh published "$(VERSION)"

import-policy-site:
	@test -d "$(SITE_REPO)" || (echo "SITE_REPO not found: $(SITE_REPO)" >&2; exit 1)
	go run ./cmd/importpolicy --repo-root "$(SITE_REPO)" --consumers backend,fixtures/second-go-host --legacy-auth-roots backend,fixtures/oidc-pet-go

.PHONY: externalconsumer-candidates externalconsumer-anonymous-local externalconsumer-public-deps-local
externalconsumer-anonymous-local:
	bash scripts/anonymous-consumer.sh local

externalconsumer-public-deps-local:
	bash scripts/anonymous-consumer.sh public-deps-local
