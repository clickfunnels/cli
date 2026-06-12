# ClickFunnels CLI (`cf`)

A terminal client for the ClickFunnels public API, written in Go on the
[Charm](https://charm.land) stack (Fang + Cobra + Lip Gloss + Huh).
The binary is `cf`. Module path: `github.com/clickfunnels/cli`.

This repo was extracted from the `clickfunnels/admin` monorepo (it used to live
at `admin/projects/cli`). It is now standalone; building and running need **only
Go** — no admin checkout, no network, no codegen toolchain.

## Quick orientation

- `make build` → `./cf`; `make install` → `$GOBIN/cf`; `make test`, `make vet`,
  `make fmt`, `make generate`.
- Requires **Go 1.26+** (uses `go tool` directives).
- Read `README.md` for usage; the **Architecture & design rationale** section
  below is the "why" (start there for design questions).

## Layout

```
cmd/                        # command layer (package cmd)
  cf/main.go                # entrypoint — wires Fang + the cobra root (the go-install target)
  root.go                   # root cmd, persistent --workspace/-w and --output/-o flags
  auth.go                   # login / logout / status (OAuth)
  teams.go contacts.go blogs.go   # curated, hand-written typed commands
  generic.go                # builds the generated command tree from the manifest
  operations.gen.go         # GENERATED operations manifest (committed) — do not edit
  api.go                    # `cf api` raw passthrough
  client.go                 # resolve account -> authed client
  format.go                 # --output flag parsing
internal/
  api/
    api.gen.go              # GENERATED models + typed client (committed) — do not edit
    client.go               # hand-written transport: auth + error normalization + cursor
    raw.go                  # RawRequest (cf api + the generic layer) + ToContactUpdate
  auth/                     # OAuth2 authorization-code + loopback + PKCE flow
  config/                   # XDG config + multi-account credential store (0600)
  output/                   # --output table/json/yaml/csv rendering
  ui/                       # shared lipgloss styles + table helper
openapi/                    # codegen config + go:generate directive
tools/specnormalize/        # spec normalizer (used by `make generate`)
tools/gencommands/          # emits cmd/operations.gen.go (used by `make generate`)
```

## The two command layers

1. **Curated** (`teams`, `contacts`, `blogs posts`): hand-written, typed,
   pretty (tables, interactive `huh` forms, validation). They call the generated
   typed client.
2. **Generated** (`generic.go` + `operations.gen.go`): the *entire* API surface
   (~262 operations, grouped by tag) built from a manifest at startup. Path
   params are flags (`--workspace-id` defaults to the active workspace),
   `--query k=v`, `--input`/`-f` for the body; routed through `api.RawRequest`.
   `cf --help` splits these into "Common commands (curated)" vs "Other API
   resources (generated…)".

`cf api [METHOD] <path>` is the raw escape hatch.

## Code generation — read this before touching generated files

`internal/api/api.gen.go` and `cmd/operations.gen.go` are **generated and
committed** (so `go build` needs no toolchain). **Never hand-edit them.** To
change them, edit the source/pipeline and run `make generate`.

`make generate` (needs `node`/`npx` + a checkout of the admin repo):

```
spec (OpenAPI 3.1, from the admin repo)         # SPEC_SRC, default ../admin/...
  -> npx @apiture/openapi-down-convert  3.1 -> 3.0   # oapi-codegen lacks 3.1 support
  -> tools/specnormalize                normalize residual JSON-Schema
  -> oapi-codegen                       -> internal/api/api.gen.go
  -> tools/gencommands                  -> cmd/operations.gen.go
```

- **SPEC_SRC** defaults to `../admin/app/views/api/v2/open_api/llm-assisted-openapi.yaml`
  (sibling checkout). Override: `make generate SPEC_SRC=/path/to/spec.yaml`.
- `openapi/openapi.gen-3.0.yaml` is a gitignored intermediate.
- `tools/specnormalize` does three load-bearing things, all to survive Go
  codegen: **strips `nullable`** (so optional fields become `*T,omitempty` —
  correct partial-update semantics; nil omits rather than nulls the field),
  **drops malformed enums**, and **collapses unions** (`[X,null]`, `oneOf`/`anyOf`
  null branches, `integer|string` id unions → first/string).
- Regeneration is idempotent: a clean `make generate` against an unchanged spec
  yields no diff.

## Key decisions & gotchas (hard-won — don't relearn these)

- **Do NOT work around upstream spec bugs in this client.** Fix them in the
  admin repo's OpenAPI spec, then regenerate — we inherit fixes automatically.
  Known upstream bugs found here: operationId naming is inconsistent
  (`createBlogPost` singular vs `listBlogsPosts` plural — the `Blogs::Post`
  convention is plural); a `locale` enum ships literal Ruby
  (`enum: I18n.available_locales.map(&:to_s)`); and there is no `limit` query
  param on contacts despite earlier assumptions. The normalizer only neutralizes
  what would break codegen — it does not "fix" naming.
- **Two login modes** (see `cmd/auth.go`):
  - **User (default):** authorize as the human; the token reaches every workspace
    they belong to. We record **one `Account` per reachable workspace**, all
    sharing that token (`Account.Installation = false`). Tied to the human's
    access.
  - **`--installation`:** the legacy workspace-scoped flow (`new_installation=true`
    → a persistent faux-user token for one chosen workspace). Records a single
    `Account` (`Installation = true`). Outlives the human's own access, so it's
    for shared/service automation, not personal use.

  Either way it's multi-account: selection (Heroku-style) is `--workspace`/`-w` →
  `CF_CLI_WORKSPACE` → the only signed-in workspace → else error, matching
  workspace **id, public id, or subdomain**. `cf auth status` shows a TYPE column
  (user/installation).
- **Config dir is `~/.config/cf` on every platform** (honors `XDG_CONFIG_HOME`,
  else `~/.config/cf`). Do NOT use Go's `os.UserConfigDir()` — on macOS it
  returns `~/Library/Application Support`, which silently broke hand-placed
  credentials once. `credentials.json` is `0600`. `CF_CLI_CONFIG_DIR` overrides.
- **Targeting a non-prod server:** `--host <domain>` swaps the base domain;
  `CF_CLI_API_BASE_URL` (or `Account.APIBaseURL`) overrides the API base entirely
  (scheme/host/port/path) — used for dev/test/CI.
- **Login flow:** OAuth is served on the workspace-agnostic **accounts host**
  (`https://accounts.<host>`), so login takes **no subdomain**. It's a loopback
  flow on a **fixed port** (8976→8977→8978) that must match the redirect URIs on
  the server's OAuth app. The default (user) flow sends no extra params and gets
  a personal token; `--installation` passes `new_installation=true` so the server
  shows a workspace picker and scopes the token to one workspace. After the token
  is issued, the CLI lists the reachable workspace(s) and stores them.
- **The CLI is a public OAuth client** (`confidential: false`), so browser login
  needs **no client secret** — and, because there's no `force_pkce`, **no PKCE
  migration is required** either. The CLI still sends PKCE on the wire
  (forward-compatible). Two client ids:
  - **Production** `clickfunnels_cli` — the default (`cmd.defaultClientID` in
    `cmd/root.go`), used against `myclickfunnels.com`. A public client id is not
    a secret, so it ships in source; every build (incl. `go install`) works with
    no setup. Overridable per login via `--client-id`/`CF_CLI_CLIENT_ID`, or at
    build time via `-ldflags "-X .../cmd.defaultClientID=<uid>"`.
  - **Dev** `hardcoded_cli_client_id` — seeded in the admin repo
    (`db/seeds/development/cli.rb`); use it with `CF_CLI_CLIENT_ID` +
    `CF_CLI_HOST=myclickfunnels.test` against a local server.
- **Local dev gotcha:** target the puma-dev `.test` host
  (`--host`/`CF_CLI_HOST=myclickfunnels.test`), NOT a Cloudflare tunnel URL —
  tunnels sit behind Cloudflare Access, which returns an HTML sign-in page to
  non-browser clients (looks like "no results"). You can `cf auth login` against
  dev with `CF_CLI_CLIENT_ID=hardcoded_cli_client_id`, or hand-write a token into
  `~/.config/cf/credentials.json` (mint one via `rails runner` /
  `Platform::AccessToken`).
- **`--output`/`-o`** is global: `table` (default), `json`, `yaml`, `csv`.
  json/yaml emit full objects; table/csv use displayed columns. `delete`
  requires `--force` when `-o` isn't `table` (no interactive prompt in scripting
  mode).

## Workflows

- Before finishing a change: `make fmt && make vet && make test` (and
  `gofmt -l .` should be empty). Cross-compile sanity: `GOOS=linux make build`.
- **Run it locally** against a dev server (see the local-dev gotcha above):
  build, write `~/.config/cf/credentials.json` with `{subdomain, host:
  "myclickfunnels.test", workspace_id, access_token}`, then `cf teams list`.
- **Adding a curated command:** mirror `cmd/contacts.go` — call the generated
  client method, render via `internal/output` with a column set. Workspace-scoped
  resources take the numeric workspace id (`account.WorkspaceID`); single-record
  routes accept the obfuscated public id.
- **Exposing more of the API generically:** it's already all there via the
  generated tree; curate a resource only when it deserves nicer UX.

## Testing

- `make test` runs the Go unit/integration suite (transport, auth flow, config
  store, output formats, command-flag logic) against in-process mock servers.
- The end-to-end test lives in the **admin repo** at `test/system/cli_test.rb`:
  it boots a real Rails server and drives a prebuilt binary via `CF_CLI_BIN`
  (skips when unset). Build `cf` here, then
  `CF_CLI_BIN=$(pwd)/cf bin/test test/system/cli_test.rb` in the admin repo.
- CI runs `gofmt`-check + build + vet + test on **RWX** on push to `main` and on
  PRs (see `.rwx/ci.yml`). `internal/ui` (lipgloss styling) is the only package
  without tests; the codegen tools and everything else are covered.

## Conventions

- Match surrounding Go style; keep comments at the altitude of *why*. Avoid dead
  code and needless abstraction.
- The thin transport (`internal/api/client.go`, `raw.go`) is intentionally
  hand-written — auth, error normalization to `*APIError`, and cursor pagination
  live there once, under the generated client. Don't push these into per-command
  code.
- Never commit built binaries (`/cf`, `/dist`) or codegen intermediates — see
  `.gitignore`.
- Do NOT add a `Co-Authored-By` / co-signature trailer to commits. Write plain
  commit messages with no Claude/AI attribution.

## Architecture & design rationale

The "why" behind the CLI, including the two questions from the kickoff:

> How do most of these tools version? Do they pull the `openapi.yaml` and
> generate endpoints from it, or keep their own copy in the client?

1. **Versioning** — the CLI versions itself with **its own SemVer git tags**,
   independent of the API version (which lives in the URL, `/api/v2`). GoReleaser
   stamps the binary at build time. See [Versioning](#versioning) below.
2. **Spec strategy** — the strong industry norm is to **generate typed code from
   the spec at dev time and commit the generated code**, *not* fetch the spec at
   runtime. We do exactly that.

### The Charm stack

Per Ben's recommendation, built on [Charm](https://charm.land):

| Library | Role |
| --- | --- |
| [`fang`](https://github.com/charmbracelet/fang) | Batteries-included Cobra wrapper — styled help/errors, `--version`, completion, manpages |
| [`cobra`](https://github.com/spf13/cobra) | Command tree (`cf auth login`, `cf teams list`, …) |
| [`lipgloss`](https://github.com/charmbracelet/lipgloss) | Declarative terminal styling (tables, colors) |
| [`huh`](https://github.com/charmbracelet/huh) | Interactive prompts (login workspace picker, `contacts create` form, delete confirms) |

The CLI is scriptable one-shot commands — no full-screen TUI. Every command
prints and exits (so it pipes cleanly); bare `cf` prints help.

### The OpenAPI question: generate-and-commit vs. fetch-at-runtime

Three options; we chose (2).

1. **Hand-write everything.** Full control, but you re-type every schema and
   drift from the spec immediately. Fine for 2 endpoints, untenable for 200.
2. **Generate typed code from the spec at dev time, commit it.** ← *what serious
   clients do, and what we do.*
3. **Fetch `openapi.yaml` at runtime and build requests dynamically.** Almost
   nobody ships this: it makes the binary depend on a network round-trip and a
   parser at startup, gives zero compile-time type safety, and turns any upstream
   spec change into a runtime break instead of a caught-at-CI break.

The whole point of a typed Go client is that `team.Name` is checked by the
compiler. Stripe, GitHub (`gh`), Twilio, Kubernetes all generate-and-commit. The
spec is pinned to the client by being baked into the committed generated code.
(`cf api` / the generic layer is the runtime-dynamic escape hatch on top.)

### Why the extra down-convert + normalize steps

Worth flagging to the API team:

- **[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) does not yet
  support OpenAPI 3.1** ([#373](https://github.com/oapi-codegen/oapi-codegen/issues/373)).
  Our spec is 3.1, so we down-convert to 3.0 first.
- After down-converting, the spec still uses JSON-Schema features that don't map
  cleanly to Go: `type: [X, "null"]` nullable arrays, `oneOf`/`anyOf` null
  branches, and `integer | string` id unions (the obfuscated-public-id /
  numeric-id pattern). `tools/specnormalize` collapses these deterministically.
- **A real upstream bug:** a `locale` enum ships literal, un-evaluated Ruby —
  `enum: I18n.available_locales.map(&:to_s)` — in *both* `openapi.yaml` and
  `llm-assisted-openapi.yaml` (so it's also live in the readme.com docs). The
  normalizer drops malformed enums so codegen survives, but **fix it upstream**.

> **Alternative considered:** the Java
> [`openapi-generator`](https://openapi-generator.tech/) supports 3.1 natively
> and would remove the down-convert step — but it needs a JVM, a heavier
> dependency than `npx` + a tiny Go normalizer. If the spec is ever served as
> clean 3.0, or oapi-codegen ships 3.1 support, the pipeline collapses to a
> single `oapi-codegen` call.

### Generated types AND client; thin hand-written transport

`oapi-codegen` generates both the **models** and a **typed client** (one method
per endpoint) into `internal/api/api.gen.go`. Nothing about the endpoints is
hand-maintained; adding one is a regen. Client generation is **scoped by tag**
(`output-options.include-tags` in `oapi-codegen.yaml`) — the full API is ~716
operations; generating all of them produces a 53k-line file that blows the repo's
1 MB file limit, so we scope to the tags we've validated and grow as needed.

What stays hand-written is the **transport** (`internal/api/client.go`, ~90
lines) — cross-cutting concerns that live in **one** place, not per endpoint:

- **Auth + error normalization** ride in an `http` *doer* the generated client
  calls (`WithHTTPClient`): it injects the bearer token and turns any non-2xx
  into a single `*APIError`, so every generated method surfaces failures through
  its plain `error` return with zero per-call status handling.
- **Pagination** is one `Cursor(*http.Response)` helper reading the
  `Pagination-Next` header.
- The `{"contact": {...}}` **envelope** and query params are *generated*, so
  they're not hand-written at all.

### Command surface: curated + generated

Two command layers, both spec-driven (this is the `gh` model — typed commands for
high-traffic paths, complete coverage for the long tail):

- **Curated** (`cmd/teams.go`, `contacts.go`, `blogs.go`): hand-built commands
  with tables, interactive `huh` forms, and validation, calling the typed client.
- **Generated** (`cmd/generic.go` + `cmd/operations.gen.go`): the entire API
  surface, one group per tag, one leaf per operation. `tools/gencommands` emits a
  compact manifest (tag, operationId, summary, method, path); `generic.go` builds
  the cobra tree from it at startup, routing through `api.RawRequest`.

We deliberately did **not** hand-write ~260 commands, nor dispatch generically to
716 heterogeneous typed methods. The manifest + `RawRequest` keeps the generic
layer tiny and fully spec-driven — when upstream fixes the `createBlogPost`
naming, a regen updates the command with no code change. Generated groups skip
tags that have a curated command (`curatedTags`) so there's one way to do each
thing.

### Correct partial updates, nested attributes, workspace scoping

- **Partial updates:** the spec's nullable fields would generate as `*T` *without*
  `omitempty`, so a nil pointer serializes to `null` and would *clear* the field
  on a `PUT`/`PATCH`. `tools/specnormalize` strips `nullable` so optional fields
  become `*T,omitempty` — send only what the caller set; reads still decode null
  to nil. Pure upside for our use case.
- **Workspace scoping:** top-level resources (teams) are account-scoped, but most
  data is nested under `/workspaces/{workspace_id}/…` needing a *numeric*
  workspace id, while single-record routes (`/contacts/{id}`) accept the
  obfuscated public id. The workspace id is resolved once at login and stored on
  the `Account`, so workspace-scoped commands need no extra round-trip.
- **Nested write data** has three flavors, handled on two layers:

  | Spec shape | Go type | CLI surface |
  | --- | --- | --- |
  | `tag_ids: [int]` (assoc array) | `*[]int` | repeatable `--tag-id 10 --tag-id 20` |
  | `custom_attributes: {k: v}` (map) | `*map[string]string` | repeatable `--attr k=v` |
  | `*_attributes` (arrays of objects) | nested structs | `--input file.json` / stdin |

  Repeatable flags for shallow arrays/maps, `--input` JSON for anything deeper
  (explicit flags overlay it), `cf api` as the final escape hatch. `*[]int`
  (not `[]int`) so `nil` "leave untouched" stays distinct from `[]` "clear all".

### Authentication

ClickFunnels uses [Doorkeeper](https://github.com/doorkeeper-gem/doorkeeper)
(OAuth2). Key facts and how they shaped the design:

- **Endpoints:** OAuth is served on the workspace-agnostic **accounts host**
  (`https://accounts.<host>`), not a workspace subdomain — so login takes **no
  subdomain**. After the token is issued the CLI lists teams/workspaces on the
  accounts host to learn what it can reach.
- **Two authorization models** (`--installation` flag):
  - **User (default):** authorize as the human (`current_user` is the token's
    resource owner). The token reaches every workspace the user belongs to and is
    bound to their access — the right model for a personal CLI. The CLI records
    one Account per reachable workspace.
  - **Installation (`--installation`):** passes `new_installation=true`, so
    `Platform::CustomConnectionWorkflow` shows a **workspace picker** and scopes
    the token to one workspace via a persistent **faux user** (the Zapier
    mechanism). The authorization **outlives the human's own access**, which is
    why it's opt-in and reserved for shared/service automation.
- **Flow:** authorization-code with a **loopback redirect** ("native app"
  pattern). The CLI binds one of a few **fixed** localhost ports
  (8976→8977→8978) — fixed because Doorkeeper matches the redirect URI *exactly*
  (including port), so those ports are pre-registered on the OAuth app.
  (`force_ssl_in_redirect_uri` exempts `localhost` but not `127.0.0.1`, so we
  advertise `http://localhost:<port>/callback`.)
- **Public client — no secret:** the first-party app is `confidential: false`
  (admin repo `db/seeds/development/cli.rb`, client id `hardcoded_cli_client_id`).
  A distributed CLI can't keep a secret, so a public client is correct:
  Doorkeeper completes the code exchange with **no client secret**, and since
  there's no `force_pkce`, **no PKCE migration is required**. We still send an
  S256 `code_challenge` on the wire — forward compatible.
- **Token lifetime:** access tokens effectively never expire (65 years) and
  refresh tokens are disabled, so the CLI stores just the access token.
- **Storage:** `~/.config/cf/credentials.json`, `0600`, isolated behind
  `internal/config` so it can be swapped for an OS keyring without touching
  callers.

**Multiple workspaces.** The CLI is multi-account either way: a user login
records one `Account` per reachable workspace (sharing the token), an
installation login records one. They accumulate in the `Store`. Active-account
selection (Heroku model): `--workspace`/`-w` → `CF_CLI_WORKSPACE` → the only
signed-in account → else error. The selector matches a workspace **id, public
id, or subdomain** (`Account.Matches`).

### Versioning

The CLI versions independently of the API.

- **API version** → URL path (`/api/v2`); the committed generated models pin the
  exact schema shape.
- **CLI version** → SemVer git tags (`v0.1.0`, …), like `gh`/`kubectl`. Build
  metadata is injected at link time into `cmd.Version/Commit/Date`.

[GoReleaser](https://goreleaser.com/) automates a release: tag `v0.1.0`, push,
and it cross-compiles macOS/Linux/Windows, stamps the version, builds archives +
checksums + Linux packages, publishes the GitHub release, and updates the
Homebrew tap. When the **API** changes, run `make generate` and cut a new CLI
release — the two version lines stay decoupled.

## Status (as of 2026-06-10)

Working end-to-end against a live dev server, including the **full browser
login** (public client, workspace picked in the browser, no secret): OAuth login
+ multi-account, curated `teams`/`contacts`(CRUD)/`blogs posts`, the full
generated surface, `cf api`, and `--output` formats.
Pushed to `github.com/clickfunnels/cli` (private); RWX CI is green;
`go install github.com/clickfunnels/cli/cmd/cf@latest` yields `cf`.

**v0.1.0 is published.** The production public client id (`clickfunnels_cli`) is
the in-code default, so the released binary authorizes against `myclickfunnels.com`
out of the box. GoReleaser ships macOS/Linux/Windows archives + Linux `.deb/.rpm/
.apk` + checksums to a GitHub Release and a Homebrew **cask** to
`clickfunnels/homebrew-tap` (`brew install clickfunnels/tap/cf`). Cut a release by
tagging `v*` (CI needs a `HOMEBREW_TAP_GITHUB_TOKEN` secret), or locally:
`GITHUB_TOKEN=$(gh auth token) HOMEBREW_TAP_GITHUB_TOKEN=$(gh auth token) goreleaser release --clean`.
macOS ships unsigned (the cask strips Gatekeeper quarantine); code signing was
deliberately deferred. The Doorkeeper PKCE migration and the spec naming/enum
fixes are **upstream in the admin repo** — nice-to-haves, not blockers.
