SHELL := /bin/bash

GORELEASER_VERSION ?= v2.18.2

.DEFAULT_GOAL := all

.PHONY: all
all: ## build pipeline
all: mod gen install build spell lint test

.PHONY: precommit
precommit: ## validate the branch before commit
precommit: check vuln

.PHONY: ci
ci: ## CI build pipeline
ci: check vuln diff

.PHONY: check
check: ## run check-only validation without changing source files
check: mod-check spell-check lint-check vet test frontend-check

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: clean
clean: ## remove files created during build pipeline
	rm -rf dist
	rm -f coverage.*
	rm -f '"$(shell go env GOCACHE)/../golangci-lint"'
	go clean -i -cache -testcache -modcache -fuzzcache -x

.PHONY: run
run: ## go run
	go run .

.PHONY: mod
mod: ## go mod tidy
	go mod tidy

.PHONY: mod-check
mod-check: ## check module files without changing them
	go mod tidy -diff

.PHONY: gen
gen: ## go generate
	go generate ./...

.PHONY: build
build: install compile

.PHONY: compile
compile:
	goreleaser build --clean --snapshot
	
.PHONY: install
install: ## install goreleaser 
	go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

.PHONY: spell
spell: ## misspell

	go tool misspell -error -locale=US -w $$(git ls-files --cached --others --exclude-standard "*.md")

.PHONY: spell-check
spell-check: ## check spelling without changing files
	go tool misspell -error -locale=US $$(git ls-files --cached --others --exclude-standard "*.md")

.PHONY: lint
lint: ## golangci-lint
	go tool golangci-lint run --fix

.PHONY: lint-check
lint-check: ## check Go lint without applying fixes
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	golangci-lint run

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: frontend-check
frontend-check: ## check frontend lint and types without changing files
	cd internal/front && npm run lint && ./node_modules/.bin/tsc -b --pretty false

.PHONY: vuln
vuln: ## govulncheck
	go tool govulncheck ./...

ifeq ($(CGO_ENABLED),0)
RACE_OPT =
else
RACE_OPT = -race
endif

.PHONY: test
test: ## go test
	go test $(RACE_OPT) -covermode=atomic -coverprofile=coverage.out -coverpkg=./... ./...
	go tool cover -html=coverage.out -o coverage.html

.PHONY: diff
diff: ## git diff
	git diff --exit-code
	RES=$$(git status --porcelain) ; if [ -n "$$RES" ]; then echo $$RES && exit 1 ; fi
