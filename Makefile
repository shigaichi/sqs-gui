.DEFAULT_GOAL := build

.PHONY: build
build:
	@go build -ldflags="-s -w" -o ./sqs-gui -trimpath ./cmd/main.go

.PHONY: dry-lint
dry-lint:
	@golangci-lint run

.PHONY: lint
lint:
	@golangci-lint run --fix
