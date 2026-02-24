CGO_ENABLED?=0
GOOS?=linux
GO_BIN?=app
GO?=go
GOFLAGS?=-ldflags=-w -ldflags=-s -a -buildvcs
MICROK8S_REGISTRY_FLAG?=SKAFFOLD_DEFAULT_REPO=localhost:32000
SKAFFOLD?=skaffold
CONFIGMAP?=deployments/kubectl/configMap.yaml
DSN?=postgresql://tenants:tenants@localhost:5432/tenants?sslmode=disable
BUF_BIN=buf


.EXPORT_ALL_VARIABLES:

# Go related
mocks: vendor
	$(GO) install go.uber.org/mock/mockgen@v0.3.0
	# generate gomocks
	$(GO) generate ./...
.PHONY: mocks

test: mocks vet
	$(GO) test ./... -cover -coverprofile coverage_source.out
	# this will be cached, just needed to the test.json
	$(GO) test ./... -cover -coverprofile coverage_source.out -json > test_source.json
	cat coverage_source.out | grep -v "mock_*" | tee coverage.out
	cat test_source.json | grep -v "mock_*" | tee test.json
.PHONY: test

test-e2e:
	cd tests/e2e && $(GO) test -v .
.PHONY: test-e2e

vet:
	$(GO) vet ./...
.PHONY: vet

vendor:
	$(GO) mod vendor
.PHONY: vendor

build:
	$(GO) build -o $(GO_BIN) ./
.PHONY: build

# Development
dev:
	./start.sh
.PHONY: dev

dev-k8s:
	$(MICROK8S_REGISTRY_FLAG) $(SKAFFOLD) run --port-forward
.PHONY: dev-k8s

clean-k8s:
	$(SKAFFOLD) delete
.PHONY: clean-k8s

# Database migrations
db-status:
	$(GO) run . migrate --dsn $(DSN) status
.PHONY: db-status

db:
	$(GO) run . migrate --dsn $(DSN) up
.PHONY: db

db-down:
	$(GO) run . migrate --dsn $(DSN) down
.PHONY: db-down

# GRPC/OpenAPI
generate:
	$(BUF_BIN) generate
.PHONY: generate

openapi-v3:
	cd convert && $(GO) run convert.go
.PHONY: openapi-v3

client-http:
	$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.1 -package httpclient -generate types,client -o client/http/client.gen.go openapi/openapi.yaml
.PHONY: client-http

