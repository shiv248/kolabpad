.PHONY: help setup install build run test clean \
        install.frontend build.frontend run.frontend test.frontend clean.frontend \
        wasm-build dev-backend dev-frontend \
        docker-up docker-down docker-restart docker-restart.debug docker-clean docker-logs \
        lint lint.frontend format format.frontend

# Variables
BINARY_NAME=kolabpad-server
BIN_DIR=bin
FRONTEND_DIR=frontend
WASM_OUTPUT=$(FRONTEND_DIR)/public/ot.wasm
WASM_EXEC=$(FRONTEND_DIR)/public/wasm_exec.js
GO_BUILD_FLAGS=-ldflags="-s -w"

# Default target
.DEFAULT_GOAL := help

help:  ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Getting started:"
	@grep -hE '^setup:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*## /|/' | awk -F'|' '{printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Development:"
	@grep -hE '^dev-.*:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*## /|/' | awk -F'|' '{printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Build:"
	@grep -hE '^build.*:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*## /|/' | awk -F'|' '{printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Testing & Quality:"
	@grep -hE '^(test|lint|format).*:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*## /|/' | awk -F'|' '{printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Cleanup:"
	@grep -hE '^clean.*:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*## /|/' | awk -F'|' '{printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Docker:"
	@grep -hE '^docker-.*:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/:.*## /|/' | awk -F'|' '{printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'

####################
# Setup & Internal #
####################

setup:  ## One-time setup for new developers
	@echo "Setting up development environment..."
	@if [ ! -f .env ]; then \
		echo "  ✓ Creating .env from .env.example"; \
		cp .env.example .env; \
	else \
		echo "  ✓ .env already exists"; \
	fi
	@echo "  ⏳ Installing Go dependencies..."
	@$(MAKE) -s install
	@echo "  ⏳ Installing frontend dependencies..."
	@$(MAKE) -s install.frontend
	@echo "  ⏳ Building WASM bridge..."
	@$(MAKE) -s wasm-build
	@echo ""
	@echo "✅ Setup complete! Start developing:"
	@echo "   Terminal 1: make dev-backend"
	@echo "   Terminal 2: make dev-frontend"

check-env:
	@if [ ! -f .env ]; then \
		echo "Error: .env file not found!"; \
		echo "Run: make setup"; \
		exit 1; \
	fi

install:
	go mod download

###################
# Backend targets #
###################

build:  ## Build backend server binary
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build $(GO_BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/server/

run: check-env
	@set -a && . ./.env && set +a && ./$(BIN_DIR)/$(BINARY_NAME)

test:  ## Run backend tests
	go test -v ./...

test.coverage:  ## Run backend tests with coverage
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

clean:  ## Clean backend build artifacts
	rm -rf $(BIN_DIR)
	rm -f server
	rm -rf dist/
	rm -f coverage.out coverage.html

####################
# Frontend targets #
####################

install.frontend:
	cd $(FRONTEND_DIR) && npm ci

build.frontend:  ## Build frontend for production
	cd $(FRONTEND_DIR) && npm run build

run.frontend:
	cd $(FRONTEND_DIR) && npm run dev

test.frontend:  ## Run frontend tests
	cd $(FRONTEND_DIR) && npm test

test.frontend.coverage:  ## Run frontend tests with coverage
	cd $(FRONTEND_DIR) && npm run test:coverage

lint.frontend:  ## Lint frontend code
	cd $(FRONTEND_DIR) && npm run lint

format.frontend:  ## Format frontend code
	cd $(FRONTEND_DIR) && npm run format

clean.frontend:  ## Clean frontend build artifacts
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/.vite
	rm -rf $(FRONTEND_DIR)/node_modules

################
# WASM targets #
################

wasm-build:
	GOOS=js GOARCH=wasm go build -o $(WASM_OUTPUT) ./cmd/ot-wasm-bridge
	cp $$(go env GOROOT)/misc/wasm/wasm_exec.js $(WASM_EXEC)

#######################
# Development targets #
#######################

dev-backend: build check-env  ## Build and run backend server
	@$(MAKE) -s run

dev-frontend:  ## Run frontend dev server
	@$(MAKE) -s run.frontend

#################
# Docker targets #
#################

docker-up:  ## Start Docker containers in background
	docker-compose up -d

docker-down:  ## Stop Docker containers
	docker-compose down

docker-restart:  ## Rebuild and restart Docker containers
	docker-compose down
	docker-compose build --no-cache
	docker-compose up

docker-restart.debug:  ## Rebuild and restart Docker with DEBUG logging
	docker-compose down
	LOG_LEVEL=DEBUG docker-compose build --no-cache
	LOG_LEVEL=DEBUG docker-compose up

docker-clean:  ## Stop containers, remove project images and data
	docker-compose down --rmi local
	rm -rf ./data/
	@echo "Docker environment cleaned (base images preserved)"

docker-logs:  ## Tail Docker container logs
	docker-compose logs -f

docker-prod-up:  ## Start Docker containers with production config (Caddy + SSL)
	@echo "🚀 Starting production Docker stack..."
	@echo "   Ensure DOMAIN and EMAIL are set in .env file"
	@echo ""
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
	@echo ""
	@echo "✅ Production stack started!"
	@echo "📜 SSL certificate will be automatically provisioned by Let's Encrypt"
	@echo ""
	@echo "Check status:"
	@echo "  make docker-prod-logs    # View logs"
	@echo "  docker ps                # Check running containers"

docker-prod-build:  ## Build and start Docker containers with production config (clean build, auto-injects git SHA)
	@GIT_SHA=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	echo "🔨 Building production Docker stack (clean build, no cache)..."; \
	echo "   Git SHA: $$GIT_SHA"; \
	echo ""; \
	VITE_SHA=$$GIT_SHA docker-compose -f docker-compose.yml -f docker-compose.prod.yml build --no-cache && \
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
	@echo ""
	@echo "✅ Production stack built and started!"

docker-prod-down:  ## Stop Docker containers (production config)
	@echo "🛑 Stopping production Docker stack..."
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml down
	@echo "✅ Production containers stopped"

docker-prod-logs:  ## Tail Docker logs (production config)
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml logs -f

docker-prod-restart:  ## Restart Docker containers (production config)
	@echo "🔄 Restarting production Docker stack..."
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml restart
	@echo "✅ Production containers restarted"

##################
# Quality targets #
##################

lint:  ## Lint and format Go code
	go vet ./...
	go fmt ./...

###################
# Combined targets #
###################

install.all:
	@$(MAKE) -s install
	@$(MAKE) -s install.frontend

build.all:  ## Build everything (WASM + backend + frontend)
	@$(MAKE) -s wasm-build
	@$(MAKE) -s build
	@$(MAKE) -s build.frontend

test.all:  ## Run all tests (backend + frontend)
	@$(MAKE) test
	@$(MAKE) test.frontend

clean.all:  ## Clean all build artifacts
	@$(MAKE) clean
	@$(MAKE) clean.frontend
	rm -f $(WASM_OUTPUT) $(WASM_EXEC)
