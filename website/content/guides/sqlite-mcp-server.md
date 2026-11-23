---
title: "MCP server for SQLite: give an AI agent a local database file"
nav_title: "SQLite"
summary: "A database file, no server, no Docker. The quickest way to see the whole thing work, and a real option for embedded and local data."
headline: "An MCP server for SQLite that runs in the same process."
description: "Connect Claude, Codex or any MCP client to a SQLite database file with DeepORG. No server and no Docker: schema discovery, a searchable catalog, one SQLite statement per question built with json_object and json_group_array, FTS5 search, and access rules enforced by the engine."
lede: "SQLite needs no server, so neither does this. Point DeepORG at a database file and your AI client can search its tables and ask questions of it, with each question compiled into one SQLite statement. The built-in demo runs this way."
weight: 60
actions:
  - label: "Connect a file"
    href: "#connect"
  - label: "Try the demo first"
    href: "#demo"
---

{{< band id="demo" label="Try it first" title="One command, no setup." >}}

The built-in demo is a SaaS company on SQLite: accounts, subscriptions, invoices and support tickets, with saved queries and workflows. It runs inside the MCP process, with nothing else to start:

```bash
curl -fsSL https://deeporg.io/install.sh | bash
claude mcp add deeporg -- deeporg mcp --demo
```

Ask your client which account is most at risk of churning. The demo is extracted to `./graphjin-demo`; delete that folder to reset it.

{{< endband >}}

{{< band id="connect" label="Connect" title="Point it at your own file." >}}

```yaml
# config/dev.yml
sources:
  - name: local
    kind: database
    type: sqlite
    default: true
    # a relative path is read from the folder DeepORG starts in
    path: /absolute/path/to/app.db
    read_only: true   # start read-only
```

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

Use absolute paths in both places: your AI client starts the process from a folder of its choosing.

{{< endband >}}

{{< band label="What you get" title="The same engine as on a server database." >}}

{{< grid >}}

- **One statement per question.** Nested results are built inside SQLite with `json_object` and `json_group_array`.
- **Foreign keys become relationships.** Declared foreign keys are discovered, so the agent follows real joins.
- **Search and geo.** Full-text search uses FTS5 tables; geo filters use SpatiaLite where it is loaded.
- **Rules still apply.** Roles, row filters and `read_only` work exactly as they do on Postgres, which makes a SQLite copy a safe place to try a policy out.

{{< endgrid >}}

{{< endband >}}

{{< band label="Limits" title="Where SQLite differs." >}}

{{< grid >}}

- **Version.** SQLite 3.25 or later, for window functions and the JSON functions the engine uses.
- **One writer.** SQLite allows one write at a time. Fine for an agent and a person; not a busy multi-user backend.
- **No array columns.** Array filters and joins are Postgres and MongoDB features.

{{< endgrid >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [Demos](/start/demos/): the other demo companies and what each one shows.
- [MCP server for PostgreSQL](/guides/postgres-mcp-server/): the five questions to ask any database MCP server.
- [MCP](/agentic/mcp/): every tool the client sees.
- [Database support](/reference/database-support/): every dialect side by side.

{{< endband >}}
