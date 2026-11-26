---
title: "Cassandra and Amazon Keyspaces AI agent: query CQL tables in plain language"
nav_title: "Cassandra and Keyspaces"
summary: "CQL-native queries that stay inside a partition, single-row writes by primary key, and polling subscriptions bound to a partition."
headline: "An AI agent for Cassandra that stays inside the partition."
description: "Connect an AI agent or MCP client to Apache Cassandra or Amazon Keyspaces with DeepORG. CQL-native queries bounded by partition keys, single-table primary-key writes, partition-bound polling subscriptions, TLS for Keyspaces, and access rules enforced by the engine."
lede: "Cassandra punishes the query a model would naturally write: a scan across every partition. DeepORG compiles the agent's questions into CQL that respects your primary keys, and refuses the shapes Cassandra cannot serve well instead of running them slowly."
weight: 90
actions:
  - label: "Connect your cluster"
    href: "#connect"
  - label: "Try the demo first"
    href: "/#demos"
---

{{< band label="The short version" title="What you get on Cassandra." >}}

{{< grid >}}

- **CQL-native queries.** A read pins the full partition key and filters clustering columns in key order, so Cassandra serves it from the partitions you named.
- **Writes by primary key.** Insert, update and delete one table's rows by their full primary key.
- **Joins by partition key.** A nested selection is fetched by the child table's full partition key, in bounded batches. Subscriptions poll a partition-bound query and report what changed.
- **Refusals that say why.** A query Cassandra cannot serve fails with the reason, such as which partition-key column is not pinned, so the agent can correct it rather than guess.

{{< endgrid >}}

{{< endband >}}

{{< band id="connect" label="Connect" title="Cassandra or Amazon Keyspaces." >}}

```bash
curl -fsSL https://deeporg.io/install.sh | bash
```

```yaml
# config/dev.yml: Apache Cassandra
database:
  type: cassandra
  connection_string: cassandra://10.0.0.1,10.0.0.2
  dbname: shop                 # the keyspace
  user: ${CASSANDRA_USER}
  password: ${CASSANDRA_PASSWORD}
  consistency: LOCAL_QUORUM    # the default
```

Amazon Keyspaces needs TLS on port 9142 and service-specific credentials. Its certificate authority is already in the system trust store:

```yaml
# config/dev.yml: Amazon Keyspaces
database:
  type: cassandra
  host: cassandra.us-east-1.amazonaws.com
  port: 9142
  dbname: shop
  user: ${KEYSPACES_USER}
  password: ${KEYSPACES_PASSWORD}
  enable_tls: true
  consistency: LOCAL_QUORUM
```

```bash
deeporg serve
```

{{< endband >}}

{{< band label="Your AI client" title="Add it to Claude Code or Codex." >}}

```bash
claude mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
codex mcp add deeporg -- deeporg mcp --path /absolute/path/to/config
```

Your client's model does the reasoning; DeepORG supplies the catalog, checks each query and runs it.

{{< endband >}}

{{< band label="Limits" title="What Cassandra does not do, on purpose." >}}

These are outside the query surface because Cassandra cannot serve them without a full scan:

{{< grid >}}

- **No aggregates.** Sums, counts and group-by belong in a warehouse or an operational SQL database.
- **No full scans.** A read that does not pin the whole partition key is refused, not run across the cluster. A known-small table can opt in with `cassandra: { allow_filtering: true }`.
- **No `or`, `not` or pattern matching.** A CQL `WHERE` is a conjunction of key conditions, so these are refused even on an opted-in table.
- **No full-text or geo search.** Use a search index or a database that has one.

{{< endgrid >}}

{{< note >}}

**Pair it with something that aggregates.** If you need totals over Cassandra data, keep a copy in a warehouse and connect both. The agent asks Cassandra for one partition and the warehouse for the totals.

{{< endnote >}}

The test suite runs against Cassandra 5.

{{< endband >}}

{{< band label="Go deeper" title="Related pages." >}}

- [Snowflake AI agent](/guides/snowflake-ai-agent/): the warehouse side of the pairing.
- [Multiple databases](/integrations/multi-database/): one query across several databases.
- [Database support](/reference/database-support/): every dialect side by side.

{{< endband >}}
