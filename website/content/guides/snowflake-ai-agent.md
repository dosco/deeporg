---
title: "Snowflake AI agent and MCP server: ask your warehouse questions in plain language"
nav_title: "Snowflake"
summary: "Key-pair sign-in, catalog discovery that pages through large accounts, and every total computed in the warehouse, not in the model."
headline: "An AI agent for Snowflake that lets the warehouse do the maths."
description: "Connect an AI agent or MCP client to Snowflake with DeepORG. Key-pair (JWT) authentication, catalog discovery that pages through large accounts, one Snowflake statement per question with aggregates computed in the warehouse, joins across Snowflake and your operational databases, and access rules enforced by the engine."
lede: "A warehouse is where a model summing rows by itself goes most wrong: the table is too large to read, so it reads a page and reports a total for the page. DeepORG has Snowflake compute every sum, count and ranking, and returns the result with the query that produced it."
weight: 70
actions:
  - label: "Connect your warehouse"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#quickstart"
---

{{< band label="The short version" title="What you get on Snowflake." >}}

{{< grid >}}

- **The warehouse computes.** Aggregates, grouping and ordering compile into the Snowflake statement. The model plans the query; it never adds up rows.
- **Big catalogs, small prompts.** Discovery pages through `SHOW` results, and the agent searches the catalog for the tables a question needs instead of reading every schema.
- **Warehouse plus app database.** A question that needs Snowflake history and today's rows from Postgres is one query; the engine runs each part where its data lives.
- **Rules from the caller.** Row filters and column lists are compiled into the statement, so an analyst's agent sees what the analyst may see.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="Key-pair sign-in, in config." >}}

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
# config/dev.yml
sources:
  - name: warehouse
    kind: database
    type: snowflake
    default: true
    # user@account/database/schema?warehouse=compute_wh&account=account
    connection_string: ${SNOWFLAKE_DSN}
    private_key_path: ${SNOWFLAKE_KEY_PATH}
    key_passphrase: ${SNOWFLAKE_KEY_PASSPHRASE}
    read_only: true   # start read-only
```

Key-pair sign-in uses an RSA-2048 key in PKCS#8 form:

```bash
openssl genrsa 2048 | openssl pkcs8 -topk8 -inform PEM -out rsa_key.p8 -nocrypt
openssl rsa -in rsa_key.p8 -pubout -out rsa_key.pub
```

Register the public key with `ALTER USER my_user SET RSA_PUBLIC_KEY='…'`. Give that user a role that can read the schemas the agent should see and use one warehouse, sized for the questions you expect.

```bash
deeporg serve
```

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code or Codex." >}}

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

Your client's model does the reasoning. To give a team one agent with a chat console and standing questions instead, run `deeporg serve` with a model key and see [the built-in agent](/agentic/server-agent/).

{{< endband >}}

{{< band label="Across systems" title="Warehouse history, live app data, one question." >}}

```yaml
sources:
  - name: app
    kind: database
    type: postgres
    connection_string: ${DATABASE_URL}
  - name: warehouse
    kind: database
    type: snowflake
    connection_string: ${SNOWFLAKE_DSN}
    private_key_path: ${SNOWFLAKE_KEY_PATH}

relationships:
  - from: warehouse.account_revenue.account_id
    to: app.accounts.id
    as: account
```

Now “revenue last year for every account with an open support ticket” is one query. Snowflake computes the revenue, Postgres finds the accounts, and the engine joins the two.

{{< endband >}}

{{< band label="Limits" title="Where Snowflake differs." >}}

{{< grid >}}

- **No live subscriptions.** Standing questions that poll for changes are for operational databases; a warehouse query on a timer is a cost you should choose on purpose.
- **Declared keys are hints.** Snowflake does not enforce foreign keys, and many warehouses declare none. Name the joins the agent should follow in config.
- **No full-text or geo filters** through the query language.
- **Every query uses your warehouse.** It resumes if suspended and bills as usual. Aggregates keep results small, but the scan is Snowflake's.

{{< endgrid >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [BigQuery AI agent](/guides/bigquery-ai-agent/): the same idea on Google's warehouse.
- [Multiple databases](/integrations/multi-database/): how a cross-database query runs.
- [Answers with evidence](/why/answers-with-evidence/): why the database computes and the model does not.
- [Database config](/configure/database/): Snowflake options in full.

{{< endband >}}
