# otel-collector-ai-lab — developer workflows.
# Run `make` or `make help` to list targets.

.DEFAULT_GOAL := help
export GOTOOLCHAIN := go1.25.11
export GOWORK := off

COMPOSE := docker compose -f deploy/compose/docker-compose.yaml
BUILDER_VERSION := v0.155.0
GO_MODULES := components services/shared services/api services/worker

help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_.-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-22s\033[0m %s\n",$$1,$$2}'

## ---------- Build ----------
tools: ## Install the OpenTelemetry Collector Builder (ocb)
	go install go.opentelemetry.io/collector/cmd/builder@$(BUILDER_VERSION)

collector: ## Build the custom otelcol-genai binary with ocb
	builder --config collector/builder-config.yaml
	@echo "built ./collector/_build/otelcol-genai"

collector-validate: collector ## Build then validate the full pipeline config
	./collector/_build/otelcol-genai validate --config collector/config/collector.yaml

build-go: ## Compile the Go services
	go -C services/api build ./...
	go -C services/worker build ./...

## ---------- Test / lint ----------
test: ## Run all Go unit tests (race + coverage)
	go -C components test -race -cover ./...
	go -C services/shared test ./... 2>/dev/null || true
	go -C services/api test ./... 2>/dev/null || true
	go -C services/worker test ./... 2>/dev/null || true

vet: ## go vet every module
	@for m in $(GO_MODULES); do echo "vet $$m"; go -C $$m vet ./...; done

lint: ## Run golangci-lint if present, else go vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		for m in $(GO_MODULES); do (cd $$m && golangci-lint run ./...); done; \
	else echo "golangci-lint not found; running go vet"; $(MAKE) vet; fi

tidy: ## go mod tidy every module
	@for m in $(GO_MODULES); do echo "tidy $$m"; go -C $$m mod tidy; done

## ---------- Run (Docker Compose) ----------
up: ## Start the full stack (collector + backends + services), building images
	$(COMPOSE) up -d --build
	@echo "API http://localhost:8080  Jaeger http://localhost:16686  Prometheus http://localhost:9090  Grafana http://localhost:3000"

load: ## Start the in-cluster load generator
	$(COMPOSE) --profile load up -d loadgen

ps: ## Show stack status
	$(COMPOSE) ps

logs: ## Tail the collector logs
	$(COMPOSE) logs -f collector

down: ## Stop the stack and remove volumes
	$(COMPOSE) --profile load down -v

loadtest: ## Fire a burst of requests at the api from the host
	bash load/loadgen.sh

## ---------- Kubernetes ----------
k8s-validate: ## Validate Kustomize overlays and the Helm chart
	kubectl kustomize deploy/k8s/overlays/dev | kubeconform -strict -summary -ignore-missing-schemas
	helm lint deploy/k8s/charts/otel-genai-collector
	helm template otel-genai deploy/k8s/charts/otel-genai-collector | kubeconform -strict -summary -ignore-missing-schemas

kind-up: ## Create a kind cluster and deploy a minimal smoke stack
	bash scripts/smoke-kind.sh up

kind-down: ## Delete the kind smoke cluster
	bash scripts/smoke-kind.sh down

## ---------- Docs / diagrams ----------
diagrams: ## Render Mermaid diagrams to SVG/PNG
	bash scripts/render-diagrams.sh

screenshots: ## Capture Grafana/Jaeger screenshots (stack must be up)
	npm install --no-save playwright && npx playwright install chromium && node scripts/screenshots.mjs

## ---------- Misc ----------
clean: ## Remove build artifacts
	rm -rf collector/_build

.PHONY: help tools collector collector-validate build-go test vet lint tidy \
	up load ps logs down loadtest k8s-validate kind-up kind-down \
	diagrams screenshots clean
