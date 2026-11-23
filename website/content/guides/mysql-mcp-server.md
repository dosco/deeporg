---
title: "MCP server for MySQL and MariaDB: connect an AI agent to MySQL"
nav_title: "MySQL and MariaDB"
summary: "MySQL 8.0+, MariaDB 10.2+, Aurora MySQL and Cloud SQL. Declare the joins your schema never wrote down, and the agent stops guessing them."
headline: "An AI agent for MySQL and MariaDB that knows how your tables join."
description: "Connect Claude, Codex or any MCP client to MySQL or MariaDB with DeepORG. Schema discovery, declared relationships for schemas without foreign keys, one parameterized statement per question, and access rules enforced by the engine. MySQL 8.0+, MariaDB 10.2+, Aurora MySQL and Cloud SQL."
lede: "DeepORG connects to MySQL 8.0+ and MariaDB 10.2+, maps the schema into a catalog the agent can search, and runs each question as one parameterized statement. Many MySQL schemas never declared their foreign keys; you can name those joins once, in config, so the agent stops guessing them."
weight: 20
actions:
  - label: "Connect your database"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#quickstart"
---

{{< band label="The short version" title="What you get on MySQL." >}}

{{< grid >}}

- **A catalog, not a prompt full of DDL.** The agent searches for the tables a question needs and reads only those.
- **One statement per question.** The engine compiles the agent's query into MySQL, with nested results built by `JSON_OBJECT` and `JSON_ARRAYAGG` inside the database.
- **The agent never writes SQL.** It writes a GraphQL query; the engine writes the statement and hands values to the driver as parameters.
- **Rules from the caller.** Row filters and column lists come from the signed-in person's role and are compiled into the statement.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="Name the database in config." >}}

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
# config/dev.yml
sources:
  - name: app
    kind: database
    type: mysql                         # or mariadb
    default: true
    # user:pass@tcp(host:3306)/app?multiStatements=true&parseTime=true&interpolateParams=true
    connection_string: ${MYSQL_DSN}
    read_only: true   # start read-only
```

The connection string is the Go driver's DSN, with the parameters the engine expects. A `mysql://` prefix is accepted and stripped. Amazon Aurora MySQL, RDS for MySQL and Google Cloud SQL for MySQL connect the same way.

```bash
deeporg serve
```

{{< endband >}}

{{< band label="Missing joins" title="Tell it the joins your schema never declared." >}}

Plenty of MySQL applications enforce relationships in code and never create a foreign key. Without one, an agent has to guess that `orders.cust_id` points at `customers.id`, and a guess is where wrong answers start.

Declare it once:

```yaml
tables:
  - name: orders
    columns:
      - name: cust_id
        foreign_key: customers.id
```

From then on the join is on the map, the agent can follow it, and the engine compiles it like any other relationship. A person can also pin a note to a column, such as which of two status columns is the real one, through [annotations](/agentic/annotations/).

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code or Codex." >}}

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

That runs DeepORG over stdio with your config; your client's model does the reasoning. Against a running server, use `deeporg mcp add claude http://localhost:8080` instead.

{{< endband >}}

{{< band label="Limits" title="Where MySQL differs." >}}

{{< grid >}}

- **No array columns.** MySQL has none, so array filters and array joins are Postgres and MongoDB features.
- **Writes run step by step.** MySQL has no `RETURNING`, so a nested insert runs as a short sequence of statements rather than a single one.
- **Geo filters need MySQL 8.0.** MariaDB has them too. Full-text search works on both.
- **Tested versions.** The test suite runs against MySQL 8.0 and MariaDB 10.11.

{{< endgrid >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [MCP server for PostgreSQL](/guides/postgres-mcp-server/): the same setup, with the five questions to ask any database MCP server.
- [DeepORG vs text-to-SQL](/compare/deeporg-vs-text-to-sql/): why a guessed join is the usual failure.
- [Relationships](/core/relationships/): how nested queries become joins.
- [Database support](/reference/database-support/): every dialect side by side.

{{< endband >}}
