# ClickFunnels CLI (`cf`)

`cf` is the command-line client for the
[ClickFunnels](https://www.clickfunnels.com) API. It lets you manage workspace
data — contacts, orders, products, funnels, courses, and the rest of the API —
from the command line.

## Install

**Homebrew:**

```bash
brew install clickfunnels/tap/cf
```

**Go:**

```bash
GOPRIVATE=github.com/clickfunnels/* \
  go install github.com/clickfunnels/clickfunnels-cli/cmd/cf@latest
```

**From source:**

```bash
git clone https://github.com/clickfunnels/clickfunnels-cli
cd clickfunnels-cli && make install   # installs `cf` to your $GOBIN
```

## Sign in

```bash
cf auth login
```

This opens your browser. Sign in and select the workspace you want to use. No
API keys to copy or paste.

To use more than one workspace, run `cf auth login` again for each one. Select
which workspace a command targets with `-w` (subdomain, id, or public id), or
set a default with `CF_CLI_WORKSPACE`:

```bash
cf auth status                 # list signed-in workspaces
cf contacts list -w acme       # target the "acme" workspace
cf auth logout -w acme         # or: cf auth logout --all
```

## Getting help

Every command has `--help`:

```bash
cf --help                      # all commands
cf contacts --help             # a resource's subcommands
cf contacts create --help      # flags for one command
```

Tab completion is available — see `cf completion --help`.

## What you can do

Some common resources have dedicated commands:

| Command | |
| --- | --- |
| `cf teams list` | List your teams |
| `cf contacts list / create / update / delete` | Manage contacts, including tags and custom attributes |
| `cf blogs posts list / create` | Manage blog posts |
| `cf api <METHOD> <path>` | Call any endpoint directly |

Every other resource in the API is available as `cf <resource> <action>`, where
`<action>` is `list`, `get`, `create`, `update`, `remove`, and so on:

- **Commerce** — `order`, `product`, `orders-invoice`, `orders-transaction`,
  `store`, `fulfillment`, `shipping-rate` / `-zone` / `-package` / `-profile`
- **Funnels & pages** — `funnel`, `page`, `funnels-split-test-step`,
  `funnels-stats`, `pages-stats`
- **Content** — `course` (+ `courses-section`, `courses-lesson`,
  `courses-enrollment`), `blog`, `image`, `site`, `theme`
- **Forms & email** — `form` (+ fields, submissions), `emails-broadcast`,
  `emails-template`, `emails-topic`
- **CRM** — `sales-opportunity`, `sales-pipeline`,
  `appointments-scheduled-event`
- **Account & automation** — `workspace`, `user`, `domain`,
  `webhooks-outgoing-endpoint`, `refine-filter`

Run `cf --help` for the complete list. Some examples:

```bash
cf order list                          # orders in the active workspace
cf product get --id <id>
cf orders-invoice list --order-id <id>
cf emails-broadcast create --input broadcast.json
```

## Output formats

Add `-o/--output` to any command: `table` (default), `json`, `yaml`, or `csv`.
The `json` and `csv` formats are useful for piping into other tools:

```bash
cf contacts list -o json | jq '.[].email_address'
cf orders list -o csv > orders.csv
```

## Configuration

Credentials and settings are stored in `~/.config/cf/` (`credentials.json`, mode
`0600`). Each setting can also be supplied by environment variable, all prefixed
`CF_CLI_`:

| Variable | Purpose |
| --- | --- |
| `CF_CLI_WORKSPACE` | Default workspace to target |
| `CF_CLI_HOST` | API host (e.g. a staging domain) |
| `CF_CLI_CLIENT_ID` | OAuth client id (for non-default servers) |
| `CF_CLI_CONFIG_DIR` | Alternate config directory |

## Contributing

Design notes, the code-generation pipeline, and internals are documented in
[CLAUDE.md](CLAUDE.md). The basics: `make build`, `make test`, `make generate`.

## License

[MIT](LICENSE) © ClickFunnels
