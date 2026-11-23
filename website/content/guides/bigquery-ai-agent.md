---
title: "BigQuery AI agent and MCP server: query BigQuery in plain language"
nav_title: "BigQuery"
summary: "Experimental and read-only. Application Default Credentials, a byte cap on every query, and totals computed in BigQuery."
headline: "An AI agent for BigQuery with a cap on what each query may scan."
description: "Connect an AI agent or MCP client to Google BigQuery with DeepORG. Experimental, read-only support: schema discovery of one dataset, Application Default Credentials, a max_bytes_billed cap on every query, aggregates computed in BigQuery, and access rules enforced by the engine."
lede: "BigQuery bills by the bytes a query scans, which makes an agent that wanders through tables an expensive one. DeepORG reads one dataset, has BigQuery compute every total, and lets you cap the bytes any single query may bill. Support is experimental and read-only."
weight: 80
actions:
  - label: "Connect your dataset"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#quickstart"
---

{{< band label="The short version" title="What you get on BigQuery." >}}

{{< grid >}}

- **Read-only by construction.** Schema discovery and queries. There are no writes, subscriptions or transactions on a BigQuery source.
- **A byte cap per query.** Set `max_bytes_billed` and BigQuery refuses any query that would bill more, before it runs.
- **BigQuery computes.** Sums, counts and grouping compile into the statement; nested results are built with `JSON_OBJECT` and `ARRAY_AGG`.
- **Rules from the caller.** Row filters and column lists from the person's role are part of every statement.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="Project, dataset, credentials." >}}

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
# config/dev.yml
sources:
  - name: analytics
    kind: database
    type: bigquery
    default: true
    connection_string: bigquery://my-project/my_dataset?location=EU&max_bytes_billed=10000000000
```

There is no host or password. Credentials come from Application Default Credentials: `GOOGLE_APPLICATION_CREDENTIALS`, the attached service account on Google Cloud, or `gcloud auth application-default login` on a workstation. Add `credentials_file=` to the connection string to use a service-account key instead.

The account needs to run query jobs in the project and read the dataset, for example the BigQuery Job User and BigQuery Data Viewer roles.

```bash
deeporg serve
```

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code or Codex." >}}

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

Your client's model plans the queries; BigQuery runs them; DeepORG checks each one against your rules first.

{{< endband >}}

{{< band label="Limits" title="Experimental, and here is what that means." >}}

{{< grid >}}

- **One dataset per source.** Discovery covers the dataset in the connection string. Add a source for each dataset the agent should see.
- **Profiling scans columns.** The catalog's sample values and value ranges come from queries that scan the columns they profile. Set `max_bytes_billed` on a large dataset.
- **Row counts may be missing.** They come from `INFORMATION_SCHEMA.TABLE_STORAGE`; where the account cannot read that view, they are left out.
- **A write preview fails.** Rehearsing a change depends on rolling back a transaction, which BigQuery does not have, so it fails instead of running.

{{< endgrid >}}

{{< note >}}

**Demos do not touch BigQuery.** Under `deeporg serve --demo`, a `type: bigquery` source runs on a local simulator, so the [demos](/start/demos/) that include one need no Google Cloud project.

{{< endnote >}}

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [Snowflake AI agent](/guides/snowflake-ai-agent/): the same idea, with writes and key-pair sign-in.
- [Multiple databases](/integrations/multi-database/): join BigQuery results to an operational database.
- [Database config](/configure/database/): every connection option.
- [Database support](/reference/database-support/): every dialect side by side.

{{< endband >}}
