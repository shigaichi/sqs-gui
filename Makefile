.DEFAULT_GOAL := build

PKGS      := $(shell go list ./...)
COVERPKG  := $(shell printf "%s" "$(PKGS)" | tr ' ' ',')
COVFILE   := coverage.out
COVHTML   := coverage.html

.PHONY: build
build:
	@go build -ldflags="-s -w" -o ./sqs-gui -trimpath ./cmd/main.go

.PHONY: test
test:
	@go test -v ./...

.PHONY: dry-lint
dry-lint:
	@golangci-lint run

.PHONY: lint
lint:
	@golangci-lint run --fix

.PHONY: test-coverage
test-coverage:
	@echo "==> Running tests with coverage (atomic mode)"
	@go test -covermode=atomic -coverpkg=$(COVERPKG) -coverprofile=$(COVFILE) $(PKGS)
	@echo
	@echo "==> Coverage summary"
	@go tool cover -func=$(COVFILE)

.PHONY: coverage-html
coverage-html: $(COVFILE)
	@echo "==> Generating HTML report: $(COVHTML)"
	@go tool cover -html=$(COVFILE) -o $(COVHTML)
	@echo "Open $(COVHTML) in your browser."
