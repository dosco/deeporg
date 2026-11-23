---
title: "Why hand-built MCP tools break: DeepORG vs a tool for every question"
nav_title: "vs hand-built MCP tools"
summary: "A tool per question works for the first ten questions. What goes wrong after that, and what to use instead."
headline: "Why hand-built MCP tools break, and what to do instead."
description: "Hand-written MCP tools, one per question, are how most teams connect an AI agent to company data. Why they stop scaling: questions outside the menu, totals the model adds up itself, schema drift, permissions re-implemented in every tool, and tool lists that crowd the prompt. How DeepORG replaces them with a searchable map and compiled queries, and when hand-built tools are still the right call."
lede: "The usual way to connect an agent to company data is to write a tool for each thing it might need: `get_customer`, `list_invoices`, `open_tickets_for_account`. It works for the first ten questions. This page is about what happens after that, and what to use instead."
weight: 10
actions:
  - label: "Try it in 2 minutes"
    href: "/#quickstart"
  - label: "No hand-built tools"
    href: "/why/no-hand-built-tools/"
---

{{< band label="The pattern" title="A tool for each question." >}}

Each tool wraps one fixed query: a name, a description for the model, a few parameters, and code that runs the query and returns rows. The agent reads the list of tools and picks one.

It is a reasonable start. The tools are easy to review, each does one thing, and the model cannot run anything that is not on the list. The trouble is that the list has to keep up with every question people ask and every change to the systems underneath, and it never does.

{{< endband >}}

{{< band label="Where it breaks" title="Five ways a tool list fails." >}}

{{< grid >}}

- **The question is not on the menu.** “Which accounts on the annual plan opened an urgent ticket this week?” needs a tool nobody wrote. The model answers with the nearest tool, or chains three and joins the results in its head.
- **The model does the arithmetic.** A tool returns a page of 50 rows. Asked for a total over 2,000, the model sums the page and reports it, confidently. The database could have computed it exactly.
- **The schema moves.** A column is renamed, a table is split, a status gains a value. Hand-written tools break, or worse, keep running and return nulls. Someone has to notice.
- **Every tool is its own security review.** Each one decides who may see what, in its own code. 43% of MCP servers tested in one 2025 study shipped with command-injection flaws.<sup><a href="#sources">1</a></sup>
- **The list crowds the prompt.** Every tool's name and description is sent with every request. In one 2025 study, a model choosing from a large pool of MCP tools picked the right one 13.62% of the time; narrowing the list first raised that to 43.13%.<sup><a href="#sources">2</a></sup>

{{< endgrid >}}

<p class="home-sources" id="sources">Sources: <a href="https://equixly.com/blog/2025/03/29/mcp-server-new-security-nightmare/" rel="nofollow">1. Equixly, 2025</a> · <a href="https://arxiv.org/abs/2505.03275" rel="nofollow">2. Gan and Sun, RAG-MCP, 2025</a></p>

{{< endband >}}

{{< band label="Instead" title="A map of everything, and the agent writes the query." >}}

DeepORG gives the agent a searchable catalog of everything you connect: tables, columns, the values a status column can take, how tables join, and the paths that cross from one database to another. The agent searches it with the person's own words, reads the entries it needs, and writes its own query.

The engine compiles that query into one database statement, with your access rules added while it compiles. The model plans; the database computes.

{{< grid >}}

- **A fixed set of tools.** Under ten MCP tools, the same for three tables or three thousand. New tables appear on the map, not in the tool list.
- **Rules written once.** Roles, row filters and column lists live in config and apply to every query the agent writes, instead of being re-coded in each tool.
- **Schema changes are discovered.** A renamed column is renamed on the map the next time the schema is read. A query that names the old one fails at compile time instead of returning nulls.
- **Totals are exact.** Sums, counts and rankings compile into the statement, so the answer covers every row, not the first page.

{{< endgrid >}}

{{< endband >}}

{{< band label="Side by side" title="The same jobs, two approaches." >}}

| | Hand-built tools | DeepORG |
| --- | --- | --- |
| A new question | Write, review and ship a tool | The agent writes the query |
| A new database | A new set of tools | A few lines of config |
| A renamed column | Find and fix every tool that used it | Rediscovered; the old name fails loudly |
| Who sees what | Re-implemented in each tool | Declared once, compiled into every query |
| A total over many rows | The model adds up a page | The database computes it |
| A write | Whatever the tool's code does | From its agent: rehearsed, held for approval, run once |
| What it sends the model | Every tool, every request | A few tools, and catalog entries on demand |

{{< endband >}}

{{< band label="Fair to both" title="When a hand-built tool is the right call." >}}

Some actions are not data questions. Issuing a refund, provisioning an account or calling a partner's API runs business logic that does not live in a table, and a reviewed, fixed action is the right shape for it.

DeepORG covers those two ways, so they stay reviewed without becoming a second tool list:

- **Workflows.** A short, reviewed script that runs steps in order, which the agent can find and call. It is rejected until the agent has read its detail.
- **API sources.** Operations from an OpenAPI spec appear on the map. Calls that change something are off until you expose them, and always wait for a person.

And if your agent only ever needs to answer three questions, three tools are fine. The map earns its place when the questions are open.

{{< endband >}}

{{< band label="Moving over" title="Keep what works." >}}

The queries behind your best tools do not have to be thrown away. Save them as [saved queries](/start/saved-queries/), and MCP clients can still call them by name, checked against the same rules. Everything else the agent can now write for itself.

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [No hand-built tools](/why/no-hand-built-tools/): how the catalog and the agent loop work.
- [DeepORG vs text-to-SQL](/compare/deeporg-vs-text-to-sql/): the other common approach, and where it breaks.
- [MCP server for PostgreSQL](/guides/postgres-mcp-server/): set it up on your own database.
- [The public benchmark](/benchmark/): how the approach scores on real organizational work, failures included.

{{< endband >}}
