---
title: "MCP server for SQL Server: connect an AI agent to Microsoft SQL Server"
nav_title: "SQL Server"
summary: "SQL Server 2012 and later. Each question compiles to one T-SQL statement built with FOR JSON PATH, inside the row rules you set."
headline: "An AI agent for SQL Server that answers in one T-SQL statement."
description: "Connect Claude, Codex or any MCP client to Microsoft SQL Server with DeepORG. Schema discovery, a searchable catalog, one T-SQL statement per question built with FOR JSON PATH, TLS settings, and role and row rules enforced by the engine. SQL Server 2012 and later."
lede: "DeepORG connects to SQL Server 2012 and later, discovers tables, keys and functions, and compiles each question the agent asks into one T-SQL statement. SQL Server builds the nested JSON itself, and your row rules are part of the statement before it runs."
weight: 30
actions:
  - label: "Connect your database"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#demos"
---

{{< band label="The short version" title="What you get on SQL Server." >}}

{{< grid >}}

- **T-SQL written by the engine.** Nested selections become `CROSS APPLY` subqueries, results are shaped with `FOR JSON PATH`, and pages use `OFFSET … FETCH NEXT`.
- **No SQL from the model.** The agent writes a GraphQL query against the catalog. The engine writes the T-SQL and passes values as parameters.
- **Rules from the caller.** A role's row filter and column list are compiled into the statement. A column a role cannot read cannot be summed either.
- **Changes wait.** When DeepORG's own agent proposes a write, it is rehearsed, rolled back and held until a person approves that exact version.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="Connection and TLS." >}}

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
# config/dev.yml
sources:
  - name: erp
    kind: database
    type: mssql
    default: true
    # sqlserver://user:pass@host:1433?database=erp
    connection_string: ${MSSQL_DSN}
    encrypt: true
    trust_server_certificate: false
    read_only: true   # start read-only
```

Keep `trust_server_certificate: false` outside a laptop: it is what makes the client check the server's certificate. For a first run, connect as a login that is a member of `db_datareader` only.

```bash
deeporg serve
```

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code or Codex." >}}

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

Your client's model does the reasoning; DeepORG supplies the catalog, the checks and the execution. With a server already running, `deeporg mcp add claude http://localhost:8080` registers it over HTTP.

{{< endband >}}

{{< band label="Ask" title="A question, a query, a statement." >}}

“Which suppliers have open purchase orders worth more than $50,000?” The agent finds `suppliers` and `purchase_orders` in the catalog, reads the relationship between them, and writes:

```graphql
query {
  purchase_orders(
    distinct: [supplier_id]
    where: { status: { eq: "open" } }
    order_by: { sum_amount: desc }
  ) {
    supplier_id
    sum_amount
    count_id
  }
}
```

SQL Server computes the totals and does the sorting. The agent reads the result, and the answer carries the rows it rests on.

{{< endband >}}

{{< band label="Limits" title="Where SQL Server differs." >}}

{{< grid >}}

- **No full-text search through the query language.** Filters, ordering, aggregates, geo filters and subscriptions work; the full-text operators are not mapped to SQL Server.
- **Bit columns are booleans.** Database functions that return a 0/1 value can be declared `return_type: boolean` under `functions:` so they come back as `true` and `false`.
- **Recursive relationships.** Supported, written as a `WITH` common table expression without the `RECURSIVE` keyword.
- **Tested version.** The test suite runs against SQL Server 2022.

{{< endgrid >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [MCP server for PostgreSQL](/guides/postgres-mcp-server/): the five questions to ask any database MCP server.
- [Your rules, your approval](/why/your-rules/): how row rules and write approvals work.
- [Database config](/configure/database/): pools, TLS and table mapping.
- [Database support](/reference/database-support/): every dialect side by side.

{{< endband >}}
