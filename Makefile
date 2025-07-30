GO_VERSION := 1.24.4

.PHONY: help build run test clean docker-build docker-run swagger deps

help: ## Show this help message
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

deps: ## Install dependencies
	go mod download
	go mod tidy

swagger: ## Generate Swagger documentation
	swag init -g cmd/server/main.go -o docs/

build: deps ## Build the application
	go build -o bin/query-assistant ./cmd/server

run: ## Run the application (local development)
	@if [ ! -f docs/docs.go ]; then echo "📝 Generating Swagger docs..."; swag init -g cmd/server/main.go -o docs/ || echo "⚠️ Swagger generation failed, continuing..."; fi
	POD_ENV=local go run ./cmd/server

run-dev: ## Run the application (dev environment - uses AWS Parameter Store)
	@if [ ! -f docs/docs.go ]; then echo "📝 Generating Swagger docs..."; swag init -g cmd/server/main.go -o docs/ || echo "⚠️ Swagger generation failed, continuing..."; fi
	POD_ENV=dev go run ./cmd/server

run-prod: ## Run the application (prod environment - uses AWS Parameter Store)
	@if [ ! -f docs/docs.go ]; then echo "📝 Generating Swagger docs..."; swag init -g cmd/server/main.go -o docs/ || echo "⚠️ Swagger generation failed, continuing..."; fi
	POD_ENV=prod go run ./cmd/server

test: ## Run tests
	go test -v ./...

test-coverage: ## Run tests with coverage
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

##@ Docker

docker-build: ## Build Docker image
	docker build -t query-assistant:latest .

docker-build-dev: ## Build Docker image for dev environment
	docker build -t query-assistant:dev .

docker-build-prod: ## Build Docker image for production
	docker build -t query-assistant:prod .

docker-run: ## Run with Docker Compose (local environment)
	@echo "🚀 Starting Docker Compose..."
	POD_ENV=local docker-compose up -d

docker-run-dev: ## Run with Docker Compose (dev environment)
	@echo "🚀 Starting Docker Compose (dev)..."
	POD_ENV=dev docker-compose up -d

docker-down: ## Stop Docker Compose
	docker-compose down

docker-logs: ## View Docker logs
	docker-compose logs -f query-assistant

##@ ClickHouse

ch-client: ## Open ClickHouse client
	docker-compose exec clickhouse clickhouse-client

ch-init: ## Initialize ClickHouse with sample data
	@echo "Creating sample tables..."
	docker-compose exec clickhouse clickhouse-client --query "CREATE TABLE IF NOT EXISTS users (user_id UInt64, name String, email String, created_at DateTime) ENGINE = MergeTree() ORDER BY user_id"
	docker-compose exec clickhouse clickhouse-client --query "CREATE TABLE IF NOT EXISTS purchases (purchase_id UInt64, user_id UInt64, amount Float64, product String, date Date) ENGINE = MergeTree() ORDER BY (date, user_id)"
	@echo "Sample tables created!"

##@ AWS Parameter Store

aws-create-dev-config: ## Create development configuration in AWS Parameter Store
	@echo "Creating development configuration in Parameter Store..."
	aws ssm put-parameter \
		--name "/dev/query-assistant" \
		--value "$$(cat scripts/dev-config.json)" \
		--type "SecureString" \
		--description "Query Assistant Development Configuration" \
		--overwrite

aws-create-prod-config: ## Create production configuration in AWS Parameter Store
	@echo "Creating production configuration in Parameter Store..."
	aws ssm put-parameter \
		--name "/prod/query-assistant" \
		--value "$$(cat scripts/prod-config.json)" \
		--type "SecureString" \
		--description "Query Assistant Production Configuration" \
		--overwrite

aws-get-dev-config: ## Get development configuration from AWS Parameter Store
	aws ssm get-parameter \
		--name "/dev/query-assistant" \
		--with-decryption \
		--query "Parameter.Value" \
		--output text | jq .

aws-get-prod-config: ## Get production configuration from AWS Parameter Store
	aws ssm get-parameter \
		--name "/prod/query-assistant" \
		--with-decryption \
		--query "Parameter.Value" \
		--output text | jq .

##@ Cleanup

clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf docs/swagger.*
	rm -f coverage.out coverage.html

clean-docker: ## Clean Docker containers and volumes
	@echo "🧹 Cleaning Docker resources..."
	docker-compose down -v || true
	docker system prune -f

##@ Quality

lint: ## Run linter
	golangci-lint run

format: ## Format code
	go fmt ./...
	goimports -w .

##@ API Testing

api-health: ## Test health endpoint
	curl -X GET http://localhost:8080/health

api-query: ## Test query endpoint (requires auth token if not skipped)
	curl -X POST http://localhost:8080/api/v1/query/execute \
		-H "Authorization: Bearer test-token" \
		-H "Content-Type: application/json" \
		-d '{"prompt":"Show me the top 10 users by total purchase amount"}'

api-schema: ## Get database schema
	curl -X GET http://localhost:8080/api/v1/schema \
		-H "Authorization: Bearer test-token"

api-docs: ## Open API documentation
	open http://localhost:8080/swagger/

##@ Development Helpers

dev-setup: ## Setup development environment
	@echo "Setting up development environment..."
	make deps
	make swagger
	@echo "Development environment ready!"

dev-reset: ## Reset development environment
	make clean
	make docker-down
	make dev-setup
	make docker-run

##@ Production

prod-build: ## Build for production
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/query-assistant ./cmd/server

deploy-dev: prod-build docker-build-dev ## Deploy to development
	@echo "Deploying to development environment..."
	# Add your deployment commands here

deploy-prod: prod-build docker-build-prod ## Deploy to production
	@echo "Deploying to production environment..."
	# Add your deployment commands here