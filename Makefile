SHELL := /bin/sh

APP := homeflow
VERSION ?= 0.15.11
REGISTRY ?= localhost:5000
IMAGE := $(REGISTRY)/homeflow/api:$(VERSION)
GO_CACHE := $(CURDIR)/.cache/go-build
GO_MOD_CACHE := $(CURDIR)/.cache/go-mod
NAMESPACE := homeflow

.DEFAULT_GOAL := help

.PHONY: check-migrations check-resource-contract mirror mirror-setup
.PHONY: help setup fmt fmt-check lint test ci mission-control mission-control-ci mission-control-install mission-control-status mission-control-logs mission-control-stop install-git-hooks frontend-test integration-test build frontend-build frontend-dev run dev down backup verify-backup docker-build docker-push kind-load manifests deploy status logs port-forward monitoring-manifests monitoring-deploy monitoring-status grafana-port-forward prometheus-port-forward grafana-password clean

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

setup: ## Check tools and require a local .env (does not install anything)
	@./scripts/check-tools.sh
	@test -f .env || { echo "Create .env from .env.example and replace all placeholders." >&2; exit 1; }

fmt: ## Format Go code
	cd backend && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) gofmt -w .
	cd integrations && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) gofmt -w .

fmt-check: ## Report Go files gofmt would rewrite; never writes
	@status=0; \
	for module in backend integrations; do \
		files=$$(cd $$module && gofmt -l .); \
		if [ -n "$$files" ]; then \
			echo "gofmt would rewrite files in $$module/:" >&2; \
			echo "$$files" | sed "s|^|  $$module/|" >&2; \
			status=1; \
		fi; \
	done; \
	if [ $$status -ne 0 ]; then \
		echo "Run 'make fmt' - a formatting run is its own claim, see Austausch.md." >&2; \
		exit 1; \
	fi; \
	echo "gofmt: nothing to rewrite"

lint: ## Run built-in Go static analysis
	cd backend && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go vet ./...
	cd integrations && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go vet ./...
	cd frontend && npm run lint

test: ## Run unit and API tests
	cd backend && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go test -race -cover ./...
	cd integrations && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go test ./...
	cd frontend && npm run test

frontend-test: ## Run web UI unit tests
	cd frontend && npm run test

integration-test: ## Run DB tests; requires HOMEFLOW_TEST_DATABASE_URL ending in _test
	@test -n "$$HOMEFLOW_TEST_DATABASE_URL" || { echo "HOMEFLOW_TEST_DATABASE_URL is required." >&2; exit 1; }
	cd backend && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go test -race -v ./internal/integration

ci: ## Local CI: gofmt, vet, TypeScript, unit tests, both manifest renders, migration and contract checks, Mission Control
	@$(MAKE) --no-print-directory fmt-check
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory test
	@$(MAKE) --no-print-directory manifests
	@$(MAKE) --no-print-directory monitoring-manifests
	@$(MAKE) --no-print-directory check-migrations
	@$(MAKE) --no-print-directory check-resource-contract
	@$(MAKE) --no-print-directory mission-control-ci
	@echo "ci: all checks passed (integration tests are separate: make integration-test)"

# Mission Control is a separate product with its own version and lifecycle, but
# it lives in this repository, so one `make ci` has to cover it too - otherwise
# a broken cockpit is only noticed by whoever opens it.
mission-control-ci: ## Run Mission Control's own checks (fmt, vet, tests, build)
	$(MAKE) -C missioncontrol ci

mission-control: ## Run the cockpit in the foreground
	$(MAKE) -C missioncontrol run

mission-control-install: ## Install the cockpit as a user service plus menu entry
	$(MAKE) -C missioncontrol install

mission-control-status: ## Show the cockpit service status
	$(MAKE) -C missioncontrol status

mission-control-logs: ## Follow the cockpit service log
	$(MAKE) -C missioncontrol logs

mission-control-stop: ## Stop the cockpit and remove its autostart
	$(MAKE) -C missioncontrol stop

install-git-hooks: ## Install the optional pre-push hook that runs 'make ci'
	./scripts/install-git-hooks.sh

build: frontend-build ## Build the local API binary and web UI
	mkdir -p bin
	cd backend && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) CGO_ENABLED=0 go build -trimpath -o ../bin/$(APP) ./cmd/homeflow

frontend-build: ## Build the static local web UI
	cd frontend && npm run build

frontend-dev: ## Run the local UI dev server; API must listen on 127.0.0.1:8080
	cd frontend && npm run dev

run: ## Run API locally; requires PostgreSQL and HomeFlow environment variables
	cd backend && GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go run ./cmd/homeflow

dev: setup ## Build and start local Compose services
	docker compose up --build

down: ## Stop Compose services without deleting data volumes
	docker compose down

backup: setup ## Back up PostgreSQL and local receipts without stopping services
	@set -a; . ./.env; set +a; ./scripts/backup.sh

verify-backup: ## Verify BACKUP_DIR checksums and archive structure
	@test -n "$(BACKUP_DIR)" || { echo "Usage: make verify-backup BACKUP_DIR=backups/<timestamp>" >&2; exit 1; }
	./scripts/verify-backup.sh "$(BACKUP_DIR)"

docker-build: ## Build the API image
	docker build -f docker/Dockerfile -t $(IMAGE) .

docker-push: ## Push to the existing local registry
	docker push $(IMAGE)

kind-load: ## Load the local image directly into kind-dev (avoids node localhost mismatch)
	@test "$$(kubectl config current-context)" = "kind-dev" || { echo "Refusing: current context is not kind-dev" >&2; exit 1; }
	kind load docker-image $(IMAGE) --name dev

manifests: ## Render Kubernetes resources locally
	kubectl kustomize kubernetes/base >/dev/null

deploy: manifests kind-load ## Deploy after the homeflow-secrets Secret was manually created
	@kubectl -n $(NAMESPACE) get secret homeflow-secrets >/dev/null 2>&1 || { echo "Missing Secret homeflow-secrets; see README." >&2; exit 1; }
	kubectl apply -k kubernetes/base

status: ## Show HomeFlow Kubernetes workloads
	kubectl -n $(NAMESPACE) get deployment,statefulset,pods,services,pvc

logs: ## Follow API logs
	kubectl -n $(NAMESPACE) logs deployment/homeflow-api --tail=100 -f

port-forward: ## Bind API only to local loopback until interrupted
	kubectl -n $(NAMESPACE) port-forward --address 127.0.0.1 service/homeflow-api 8080:80

monitoring-manifests: ## Render local monitoring resources without applying them
	kubectl kustomize monitoring/kubernetes >/dev/null

monitoring-deploy: monitoring-manifests ## Deploy local-only Prometheus, Grafana and kube-state-metrics
	@test "$$(kubectl config current-context)" = "kind-dev" || { echo "Refusing: current context is not kind-dev" >&2; exit 1; }
	@./scripts/ensure-monitoring-secret.sh
	kubectl apply -k monitoring/kubernetes

monitoring-status: ## Show local monitoring workloads and storage
	kubectl -n homeflow-monitoring get deployments,pods,services,pvc

grafana-port-forward: ## Open Grafana only on local loopback until interrupted
	kubectl -n homeflow-monitoring port-forward --address 127.0.0.1 service/grafana 3000:3000

prometheus-port-forward: ## Open Prometheus only on local loopback until interrupted
	kubectl -n homeflow-monitoring port-forward --address 127.0.0.1 service/prometheus 9090:9090

grafana-password: ## Intentionally display the local Grafana admin password
	@kubectl -n homeflow-monitoring get secret grafana-admin -o jsonpath='{.data.admin-password}' | base64 -d; echo

clean: ## DESTRUCTIVE only for generated build/cache files; preserves DB and receipts
	rm -f bin/homeflow
	GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go clean -cache -testcache -modcache 2>/dev/null || true

check-migrations: ## Check migration numbering against contracts.md 1.3 (no database needed)
	@echo "check-migrations:"
	@./scripts/check-migrations.sh

check-resource-contract: ## Check contracts.md 2.1 against the rendered manifests and against allocatable
	@echo "check-resource-contract:"
	@./scripts/check-resource-contract.sh

mirror: ## Push the history to every configured mirror remote (see ADR 0008)
	@./scripts/mirror-repo.sh

mirror-setup: ## Create the local mirror, register it and install the hooks (one command)
	@./scripts/mirror-setup.sh
