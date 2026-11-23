---
title: "Text-to-SQL vs an AI agent: DeepORG vs Vanna-style text-to-SQL"
nav_title: "vs text-to-SQL"
summary: "Text-to-SQL has the model write SQL from a schema it was shown. What changes when the model writes against a map and an engine writes the SQL."
headline: "Text-to-SQL vs an agent: what changes when the model stops writing SQL."
description: "Text-to-SQL tools such as Vanna retrieve schema, documentation and example queries, then have a model write SQL. How that compares with DeepORG, where the agent searches a catalog, writes a query the engine compiles into SQL with your access rules, and works across databases. Where text-to-SQL is the better choice, and where it breaks."
lede: "Text-to-SQL hands a model your schema and asks it for SQL. Vanna made the approach popular: retrieve the table definitions, documentation and past queries closest to the question, then have the model write the statement. DeepORG takes a different line. The model writes against a map, and an engine writes the SQL."
weight: 20
actions:
  - label: "Try it in 2 minutes"
    href: "/#quickstart"
  - label: "Answers with evidence"
    href: "/why/answers-with-evidence/"
---

{{< band label="How text-to-SQL works" title="Retrieve context, write SQL, run it." >}}

A Vanna-style system is trained, in its own sense of the word, by loading three kinds of context into a vector store: table definitions (DDL), free-text documentation of business terms, and pairs of questions with the SQL that answers them. When a question comes in, it retrieves the closest entries, puts them in the prompt, and the model writes a SQL statement. The application runs that statement on a database connection and shows the result, often as a table or a chart.

It has real strengths. The database does the arithmetic. The SQL is there to read, so an analyst can check it or paste it somewhere else. And a good library of example queries goes a long way on a warehouse with a stable set of questions.

The open-source Vanna repository has since been archived by its owners, but the approach is widespread and many tools work the same way.

{{< endband >}}

{{< band label="Where it breaks" title="The hard part is not the SQL syntax." >}}

{{< grid >}}

- **Real schemas are hard.** 21% is the best model's success rate on real enterprise text-to-SQL, down from 91% on academic benchmarks.<sup><a href="#sources">1</a></sup> Hundreds of tables, cryptic names and several dialects are the normal case, not the edge.
- **Joins are guessed.** The model sees column names and infers which ones match. `orders.cust_id` to `customers.id` is a guess unless something says so.
- **Permissions are the connection's.** The statement runs as whatever database user the tool holds. Restricting rows per person means building that yourself, around SQL the model wrote.
- **Raw SQL is a wide door.** Read-only depends on the database user, or on a wrapper that inspects text. A wrapper that inspects text can be talked past.<sup><a href="#sources">2</a></sup>
- **The examples go stale.** Question-and-SQL pairs encode the schema as it was. When it changes, the examples teach the model the old one.
- **One database per question.** A question that needs the warehouse and the app database is two systems the model has to reconcile by itself.

{{< endgrid >}}

<p class="home-sources" id="sources">Sources: <a href="https://arxiv.org/abs/2411.07763" rel="nofollow">1. Spider 2.0, ICLR 2025</a> · <a href="https://securitylabs.datadoghq.com/articles/mcp-vulnerability-case-study-SQL-injection-in-the-postgresql-mcp-server/" rel="nofollow">2. Datadog Security Labs, 2025</a></p>

{{< endband >}}

{{< band label="What changes" title="The model plans. The engine writes the SQL." >}}

In DeepORG the agent does not write SQL. It searches a catalog of everything you connect, reads the entries a question needs, and writes a GraphQL query against it. The engine compiles that query into one statement for whichever database holds the data, Postgres, SQL Server, Oracle, Snowflake or another, with your access rules added while it compiles.

{{< grid >}}

- **Joins come from the map.** Relationships are discovered from foreign keys or declared once in config. The agent follows a path that exists; it cannot invent one.
- **Wrong names fail early.** An unknown table or column fails at compile time, with a pointer to the real one, before anything reaches the database.
- **Rules belong to the person.** Identity comes from the caller's token, and their role's row filters and column lists are part of the statement.
- **No SQL text to smuggle.** Values travel as parameters, and nothing in a query can append a second SQL statement.
- **One question, several databases.** The engine runs each part where its data lives and joins the results.
- **More than one shot.** The agent loop is discover, check, validate, act. A failed query comes back with a reason, and the agent tries again with it.

{{< endgrid >}}

An answer comes back with the rows it rests on and a record of what ran. A figure in the answer that no query produced is blocked, not shown.

{{< endband >}}

{{< band label="Side by side" title="Two ways to answer a question from data." >}}

| | Text-to-SQL | DeepORG |
| --- | --- | --- |
| What the model writes | SQL, in the database's dialect | A GraphQL query against the catalog |
| How it learns the schema | Retrieved DDL, docs and example SQL | Catalog search, read on demand |
| Joins | Inferred from names and examples | Discovered or declared relationships |
| Who sees what | The connection's database user | The caller's role, compiled into each query |
| Several databases | One per question | One query, run where each part lives |
| Writes | Whatever the SQL does, if the user may | Held for a person's approval by default |
| Evidence | The SQL and its result | The rows, the queries that ran, ungrounded figures blocked |
| How good is it | Your own testing | A [public benchmark](/benchmark/), failures included |

{{< endband >}}

{{< band label="Fair to both" title="When text-to-SQL is the better choice." >}}

- **The SQL is the product.** If analysts want a statement to paste into a notebook or a BI tool, a tool that writes SQL gives them exactly that.
- **One warehouse, a known set of questions.** With a curated library of examples and a read-only user, text-to-SQL can do well and is simple to run.
- **You need SQL the query language cannot say.** DeepORG's queries cover filters, joins, grouping, aggregates, rankings and database functions, not every construct a dialect has. A one-off shape outside that is easier to write as SQL, or to put behind a view or function that DeepORG can then use.

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [Answers with evidence](/why/answers-with-evidence/): what comes back with every answer.
- [DeepORG vs hand-built MCP tools](/compare/deeporg-vs-hand-built-mcp-tools/): the other common approach.
- [The compiler](/core/compiler/): how one query becomes one statement.
- [Snowflake AI agent](/guides/snowflake-ai-agent/) and [MCP server for PostgreSQL](/guides/postgres-mcp-server/): set it up on your own data.

{{< endband >}}
