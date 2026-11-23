---
title: "MCP server for PostgreSQL: connect Claude, Codex or any AI agent to Postgres"
nav_title: "PostgreSQL"
summary: "Also Aurora, RDS, Cloud SQL, CockroachDB and YugabyteDB. The agent writes queries, the engine runs them as one Postgres statement inside your access rules."
headline: "An MCP server for PostgreSQL that writes its own queries inside your rules."
description: "Connect Claude, Codex or any MCP client to PostgreSQL with DeepORG: schema discovery, a searchable catalog, one parameterized SQL statement per question, role and row rules enforced by the engine, and writes held for approval. Works with Aurora, RDS, Cloud SQL, CockroachDB and YugabyteDB."
lede: "DeepORG reads your Postgres schema, gives your AI client a small fixed set of tools, and turns each question into one parameterized SQL statement that runs inside the access rules you set. It works the same on RDS, Aurora, Cloud SQL, CockroachDB and YugabyteDB."
weight: 10
actions:
  - label: "Connect your database"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#quickstart"
---

{{< band label="The short version" title="What you get when you point it at Postgres." >}}

DeepORG connects to PostgreSQL, discovers the tables, columns, foreign keys and functions, and builds a catalog the agent can search. Your AI client sees the same small set of tools whether the database has three tables or three thousand.

{{< grid >}}

- **The agent never sends SQL.** It writes a GraphQL query against the catalog. The engine compiles it into one Postgres statement and passes every value as a parameter.
- **Joins come from your foreign keys.** The relationships are discovered, not guessed, so the agent does not invent a join column.
- **Postgres does the maths.** Sums, counts, averages, grouping and ranking run in the database, not in the model's head over a page of rows.
- **Your rules apply at compile time.** Roles, row filters and column lists are added to the statement before it runs. A read-only source stays read-only.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="A few lines of config." >}}

Install the CLI, then name the database in `config/dev.yml`:

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
sources:
  - name: app
    kind: database
    type: postgres
    default: true
    # postgres://user:pass@host:5432/app?sslmode=require
    connection_string: ${DATABASE_URL}
    schema: public
    read_only: true   # start read-only
```

`read_only: true` blocks every write to the source, whatever a role or the agent asks for. For a first run, also connect as a Postgres user that only has `SELECT`, so the database itself enforces the same limit.

Start it and check that it found your tables:

```bash
deeporg serve
```

Open `localhost:8080/admin/runtime` to see the discovered source and table count.

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code, Codex or any MCP client." >}}

Run it over stdio, pointed at your config folder. Nothing else needs to be running:

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

Or, with `deeporg serve` already running, register its HTTP endpoint:

```bash
deeporg mcp add claude http://localhost:8080
```

Your client's own model does the reasoning, so DeepORG needs no model key in this setup. To use DeepORG's own agent instead, with a chat console and standing questions, see [the built-in agent](/agentic/server-agent/).

{{< endband >}}

{{< band label="What runs" title="A question becomes one Postgres statement." >}}

Ask “who were our five biggest customers last quarter?” The agent searches the catalog for customers and orders, reads how they join, and writes:

```graphql
query {
  orders(
    distinct: [customer_id]
    where: { created_at: { gte: "2026-07-01", lt: "2026-10-01" } }
    order_by: { sum_total: desc }
    limit: 5
  ) {
    customer_id
    sum_total
    count_id
  }
}
```

The engine compiles that into a single statement: a `GROUP BY` with the sum and count, the role's row filter added to the `WHERE`, and the dates passed as parameters. Nested selections become `LATERAL` joins, and Postgres builds the JSON response itself, so there is one round trip and no stitching in application code.

On Postgres the query language reaches the most: arrays, full-text search, PostGIS geo filters, recursive relationships, JSON columns, database functions and live subscriptions. See [the compiler](/core/compiler/) for how the statement is built.

{{< endband >}}

{{< band label="Choosing a server" title="Five questions to ask any Postgres MCP server." >}}

Plenty of MCP servers will connect a model to Postgres. Before you give one your production database, check these:

1. **Does the model write raw SQL?** If so, “read-only” is only as strong as the wrapper. The original reference Postgres MCP server ran each query in a read-only transaction, and a `COMMIT` inside the query string ended that transaction early, so the statements after it could write.<sup><a href="#sources">1</a></sup>
2. **Where do permissions come from?** A shared database user means every caller sees everything that user sees. Look for row rules tied to the signed-in person.
3. **How does it learn the schema?** Dumping every table into the prompt stops working past a few dozen tables. Search on demand scales; a full dump does not.
4. **Who does the arithmetic?** If the model sums a page of rows itself, a total over more rows than the page is wrong, confidently.
5. **What happens on a write?** It should be refused, or held for a person to approve, never run because the model decided to.

DeepORG's answers: GraphQL compiled with parameters, rules from the caller's token, catalog search, aggregates in the database, a read-only switch per source, and writes from its own agent held as [approvals](/why/your-rules/).

<p class="home-sources" id="sources">Source: <a href="https://securitylabs.datadoghq.com/articles/mcp-vulnerability-case-study-SQL-injection-in-the-postgresql-mcp-server/" rel="nofollow">1. Datadog Security Labs, SQL injection in the Postgres MCP server, 2025</a></p>

{{< endband >}}

{{< band label="Postgres-compatible" title="RDS, Aurora, Cloud SQL, CockroachDB, YugabyteDB." >}}

These connect with `type: postgres` and their usual connection string.

{{< grid >}}

- **Amazon RDS and Aurora.** Use the cluster endpoint, or a reader endpoint for an agent that only reads.
- **Google Cloud SQL.** Connect through the Cloud SQL Auth Proxy, or set `enable_tls` with `server_name` as `project:instance`.
- **CockroachDB.** Queries, writes, subscriptions, arrays and full-text search. No PostGIS geo filters and no schema management.
- **YugabyteDB.** Uses the Postgres-compatible YSQL API, through the same driver.

{{< endgrid >}}

{{< endband >}}

{{< band label="Limits" title="What to know before you start." >}}

{{< note >}}

**A question that crosses two databases is not one statement.** If orders live in Postgres and customers in another database, the engine fetches the parent rows, then runs the child query on the other database and joins the results.

{{< endnote >}}

{{< note >}}

**Order is not guaranteed without `order_by`.** Postgres often returns rows in insert order, but that is not a promise, and the engine does not add an ordering you did not ask for.

{{< endnote >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [DeepORG vs hand-built MCP tools](/compare/deeporg-vs-hand-built-mcp-tools/): why a tool per question stops scaling.
- [DeepORG vs text-to-SQL](/compare/deeporg-vs-text-to-sql/): what changes when the model stops writing SQL.
- [Database config](/configure/database/): pools, TLS and table mapping.
- [Access rules](/configure/auth-rbac/): roles, row filters and column lists.
- [MCP](/agentic/mcp/): every tool and how hosted sign-in works.

{{< endband >}}
