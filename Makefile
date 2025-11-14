.PHONY: lint build swag help

help:
	@echo "Available targets:"
	@echo "  make lint - Run golangci-lint and govulncheck"
	@echo "  make swag   - Generate Swagger documentation"
	@echo "  make build  - Build Docker image ghcr.io/talentabangsa/govid:develop"

lint:
	@echo "Running linter..."
	golangci-lint run
	@echo "Running vulnerability check..."
	govulncheck ./...
	@echo "Linting complete!"

swag:
	@echo "Generating Swagger documentation..."
	swag init -g cmd/main.go --outputTypes yaml --parseDependency --parseInternal
	@echo "Swagger documentation generated!"

build:
	@echo "Building Docker image..."
	docker build -t ghcr.io/talentabangsa/govid:develop . && dkr --push ghcr.io/talentabangsa/govid:develop
	@echo "Build complete!"
