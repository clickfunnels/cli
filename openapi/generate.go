// Package openapi anchors the code-generation directive for the API client.
//
// `make generate` reads the spec from a checkout of the admin (ClickFunnels
// Rails) repo (SPEC_SRC, default ../admin), down-converts 3.1 -> 3.0, normalizes
// residual JSON-Schema unions (see ../tools/specnormalize), and writes the
// codegen-clean openapi.gen-3.0.yaml here (gitignored). This directive turns
// that into the typed models + client.
//
// Run `make generate` to refresh from the spec. The resulting
// internal/api/api.gen.go is committed, so a plain `go build` never needs
// the codegen toolchain, node, or network access.
package openapi

//go:generate go tool oapi-codegen -config oapi-codegen.yaml openapi.gen-3.0.yaml
