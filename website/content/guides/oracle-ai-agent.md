---
title: "Oracle AI agent and MCP server: query Oracle Database in plain language"
nav_title: "Oracle"
summary: "Oracle Database through one compiled SQL statement per question, with JSON built by JSON_ARRAYAGG and booleans handled for you."
headline: "An AI agent for Oracle Database that writes no SQL of its own."
description: "Connect an AI agent or MCP client to Oracle Database with DeepORG. Schema discovery, a searchable catalog, one Oracle SQL statement per question built with JSON_OBJECT and JSON_ARRAYAGG, FETCH FIRST paging, boolean handling for NUMBER results, and access rules enforced by the engine."
lede: "Oracle schemas tend to be old, large and full of names nobody remembers. DeepORG maps them into a catalog the agent searches by intent, then compiles each question into one Oracle statement that runs inside the access rules you set."
weight: 40
actions:
  - label: "Connect your database"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#quickstart"
---

{{< band label="The short version" title="What you get on Oracle." >}}

{{< grid >}}

- **Search by intent.** “Late shipments” finds the shipment tables and their status values, whatever the columns are called. The agent reads the matching entries, not the whole data dictionary.
- **Oracle SQL written by the engine.** Nested results are built with `JSON_OBJECT` and `JSON_ARRAYAGG`, and pages use `FETCH FIRST n ROWS ONLY`.
- **Notes on the map.** A person can pin a note to a cryptic column (`STAT_CD` means order status, `9` means cancelled) and the agent reads it as data.
- **Rules from the caller.** Row filters and column lists are compiled into the statement, not left to the model.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="Name the database in config." >}}

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
# config/dev.yml
sources:
  - name: ops
    kind: database
    type: oracle
    default: true
    # oracle://user:pass@host:1521/SERVICE_NAME
    connection_string: ${ORACLE_DSN}
    read_only: true   # start read-only
```

For a first run, connect as a user that has `SELECT` on the schemas the agent should see and nothing else.

```bash
deeporg serve
```

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code or Codex." >}}

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

DeepORG runs over stdio with your config, and your client's model does the reasoning. Against a running server, `deeporg mcp add claude http://localhost:8080` registers it over HTTP.

{{< endband >}}

{{< band label="Oracle specifics" title="The details the engine handles." >}}

{{< grid >}}

- **Booleans.** Oracle functions return `NUMBER` where other databases return a boolean. Declare the return type once and the result comes back as `true` or `false`.
- **Identifier case.** Oracle stores unquoted names in upper case. The catalog shows the names as they are, and the engine quotes them correctly.
- **Recursive relationships.** Written as a `WITH` clause without the `RECURSIVE` keyword, which Oracle does not use.
- **Tested version.** The test suite runs against Oracle Database Free 23.

{{< endgrid >}}

```yaml
functions:
  - name: is_active
    return_type: boolean
```

{{< endband >}}

{{< band label="Limits" title="Where Oracle differs." >}}

{{< note >}}

**No full-text search and no array columns through the query language.** Filters, ordering, aggregates, geo filters, writes and subscriptions work. Oracle Text indexes are not mapped to the search operators.

{{< endnote >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [DeepORG vs text-to-SQL](/compare/deeporg-vs-text-to-sql/): why a model writing Oracle SQL from a schema dump goes wrong.
- [Annotations](/agentic/annotations/): notes people pin to the catalog.
- [MCP server for PostgreSQL](/guides/postgres-mcp-server/): the five questions to ask any database MCP server.
- [Database support](/reference/database-support/): every dialect side by side.

{{< endband >}}
