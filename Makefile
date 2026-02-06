# ==============================================================================
# Development
# ==============================================================================
##@ Development

.PHONY: deps
deps: ## Download dependencies
	go mod download

.PHONY: tidy
tidy: ## Tidy up go.mod
	go mod tidy

.PHONY: fmt
fmt: ## Format code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: fmt vet ## Run fmt, vet, and golangci-lint
	golangci-lint run ./...

.PHONY: test
test: ## Run unit tests with race detector and coverage (internal only)
	@go test -v -race -coverprofile=coverage.out ./internal/...
	@go tool cover -func=coverage.out
	@rm coverage.out

.PHONY: ci
ci: lint test ## Run all CI steps (lint + test)

# ==============================================================================
# Code Generation
# ==============================================================================
##@ Code Generation

.PHONY: gen-proto
gen-proto: ## Generate Protobuf & gRPC code
	@echo "Generating Protobuf code..."
	@protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/ledger.proto
	@echo "Done!"