BINARY := cf
PKG := github.com/clickfunnels/clickfunnels-cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(PKG)/cmd.Version=$(VERSION) -X $(PKG)/cmd.Commit=$(COMMIT) -X $(PKG)/cmd.Date=$(DATE)

.PHONY: build install test vet fmt run generate clean

build: ## Build the cf binary into ./cf
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/cf

install: ## Install cf into $GOBIN
	go install -ldflags "$(LDFLAGS)" ./cmd/cf

test: ## Run tests
	go test ./...

vet: ## go vet
	go vet ./...

fmt: ## gofmt the tree
	gofmt -w .

run: build ## Build and run
	./$(BINARY)

# --- OpenAPI codegen pipeline -------------------------------------------------
# Only the generated api.gen.go + operations.gen.go are committed, so plain
# `go build` needs no toolchain, network, or the spec. Regeneration reads the
# spec from a checkout of the admin (ClickFunnels Rails) repo. SPEC_SRC defaults
# to a sibling checkout (../admin); override it for a different location, e.g.
#   make generate SPEC_SRC=/path/to/admin/app/views/api/v2/open_api/llm-assisted-openapi.yaml
#
# spec (3.1, from the admin repo)
#   -> down-convert to 3.0   (npx @apiture/openapi-down-convert)
#   -> normalize             (tools/specnormalize: enums, nullable/scalar unions)
#   -> oapi-codegen          -> internal/api/api.gen.go (models + client, committed)

SPEC_SRC ?= ../admin/app/views/api/v2/open_api/llm-assisted-openapi.yaml

generate: ## Regenerate the typed client from the admin repo's OpenAPI spec (SPEC_SRC)
	npx -y @apiture/openapi-down-convert@latest --input $(SPEC_SRC) --output openapi/openapi.30.tmp.yaml
	go run ./tools/specnormalize openapi/openapi.30.tmp.yaml openapi/openapi.gen-3.0.yaml
	cd openapi && go tool oapi-codegen -config oapi-codegen.yaml openapi.gen-3.0.yaml
	gofmt -w internal/api/api.gen.go
	go run ./tools/gencommands openapi/openapi.gen-3.0.yaml cmd/operations.gen.go
	gofmt -w cmd/operations.gen.go
	rm -f openapi/openapi.30.tmp.yaml

clean:
	rm -f $(BINARY)
	rm -rf dist
