---
title: "MCP server for MongoDB: connect an AI agent to MongoDB and Atlas"
nav_title: "MongoDB"
summary: "MongoDB and Atlas. The agent writes the same queries it writes for SQL; the engine turns them into aggregation pipelines with $lookup."
headline: "An AI agent for MongoDB that writes queries, not pipelines."
description: "Connect Claude, Codex or any MCP client to MongoDB or Atlas with DeepORG. The agent writes one query language for every database; the engine translates it into MongoDB aggregation pipelines, joins collections with $lookup, and enforces access rules. Declared relationships, embedded documents, cursors and subscriptions."
lede: "On MongoDB the agent writes the same queries it writes for Postgres. The engine turns each one into an aggregation pipeline: `$match` for filters, `$lookup` for relationships, `$project` for the fields asked for. One query language covers your documents and your SQL databases."
weight: 50
actions:
  - label: "Connect your database"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#demos"
---

{{< band label="The short version" title="What you get on MongoDB." >}}

{{< grid >}}

- **Pipelines written by the engine.** Filters, ordering, limits, relationships and embedded arrays compile into one aggregation pipeline. The model never builds a pipeline by hand.
- **Joins across collections.** A nested selection becomes a `$lookup` with its own pipeline. An array of ids joins with `$in`.
- **One language across databases.** A question that spans MongoDB and Postgres is written once; the engine runs each part on its own database.
- **Rules from the caller.** Row filters from the signed-in person's role are added to the `$match` stage.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="Connection, database and relationships." >}}

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
# config/dev.yml
sources:
  - name: docs
    kind: database
    type: mongodb
    default: true
    # mongodb+srv://user:pass@cluster.example.net
    connection_string: ${MONGODB_URI}
    dbname: app                         # required: the database to read
    read_only: true   # start read-only
```

Name the database in `dbname`; the one in the URI is not used. MongoDB has no foreign keys, so tell the engine how collections relate:

```yaml
tables:
  - name: orders
    columns:
      - name: customer_id
        foreign_key: customers.id
```

The agent then follows `orders → customer` as it would on any SQL database. Fields are addressed as `id`; the engine maps them to `_id`.

```bash
deeporg serve
```

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code or Codex." >}}

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

Your client's model does the reasoning. With a server already running, `deeporg mcp add claude http://localhost:8080` registers it over HTTP.

{{< endband >}}

{{< band label="What runs" title="A nested query becomes $lookup." >}}

```graphql
query {
  orders(where: { status: { eq: "open" } }, order_by: { id: desc }, limit: 10) {
    id
    total
    customer { id name }
  }
}
```

The engine sends MongoDB one pipeline: a `$match` on status plus the role's filter, a `$sort` and `$limit`, and a `$lookup` into `customers` that projects only `id` and `name`. Embedded arrays queried as their own table use `$unwind` and `$group` so the parent document keeps its shape.

{{< endband >}}

{{< band label="Limits" title="Where MongoDB differs." >}}

{{< grid >}}

- **Some SQL features do not carry over.** Counts, sums and averages run as a `$group` stage, but analytics directives (window-style rankings) and database functions are SQL-only. Check before you rely on an aggregate-heavy question being portable.
- **Relationships are declared.** Nothing is inferred from field names; a missing declaration means a missing join.
- **No schema management.** Collections are discovered from your data; the engine does not create or migrate them.
- **Tested version.** The test suite runs against MongoDB 7.

{{< endgrid >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [MongoDB in the docs](/integrations/mongodb/): the pipeline format, cursors and ids.
- [Multiple databases](/integrations/multi-database/): one query across MongoDB and SQL.
- [DeepORG vs hand-built MCP tools](/compare/deeporg-vs-hand-built-mcp-tools/): why a tool per collection stops scaling.
- [Database support](/reference/database-support/): every dialect side by side.

{{< endband >}}
