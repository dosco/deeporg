---
title: "DeepORG — The Organizational Agent Benchmark"
description: "A public comparison of AI agents doing real organizational work, with correctness, consistency, safety, cost, latency, and DeepORG build provenance."
aliases:
  - /benchmarks/organizational-agent/
---

DeepORG runs each model as an agent against a live company system, not a list
of stored answers. Every task is checked for the right answer, a database-side
method, the expected behavior and zero unsafe effects. Each row links to the
exact test version and DeepORG build behind it.

## Model comparison

Each model shows its best trustworthy published result. Rows also score three
groups: **questions** (answers computed from live data), **operations** (writes,
watches, multi-source work, and follow-ups before test version `2028.6`) and
**governance** (refusing what policy forbids). A model can be a strong analyst
and a weak operator.

{{< benchmark-leaderboard benchmark="deeporg" view="chart" >}}

{{< benchmark-leaderboard benchmark="deeporg" view="models" >}}

## Operational cost

The same attempts as the chart above, measured in tokens, time and list-price
cost. Lower is better.

{{< benchmark-leaderboard benchmark="deeporg" view="operational" >}}

## Where each model is strong

Scores by task family, for models run on the current test version.

{{< benchmark-leaderboard benchmark="deeporg" view="categories" >}}

## How scoring works

| Dimension | What it checks |
| --- | --- |
| **Correct answer** | The final value matches a hidden answer computed from the live system. |
| **Required method** | The action trail shows the database or DeepORG did the work. |
| **Full pass** | Correct answer, required method, expected behavior and safety all pass. |
| **Passed every attempt** | All three attempts earned a full pass. |
| **Efficiency** | Tokens, p50/p95 latency and list-price cost, reported separately. |

The [methodology](/benchmarks/deeporg/methodology/) covers scoring,
comparability, corrections and privacy. Every published run is in the
[run archive](/benchmarks/deeporg/runs/).

## Run it yourself

The public suite is frozen and ships with DeepORG. Each task runs three times
against the live demo. Running it costs only provider tokens.

{{< code-card filename="terminal" language="bash" >}}
deeporg eval bench --public --yes
deeporg eval publish <run-id> --benchmark deeporg --yes
{{< /code-card >}}

To check a setup before paying for a full run, `deeporg eval bench --quick`
runs 25 of the suite's tasks once each, about 7% of the cost. The 25 are the
tasks whose past results best follow a run's full score, with at least two from
every task family. A quick score moves about 6 points from one run to the next,
so it catches a broken setup or a large drop, not a small difference. Quick runs
are never published.

Publishing writes one data row and one report page for you to review and
commit. Low scores are valid results too.

## Run DeepORG on your organization

The same generator builds a private exam from your own DeepORG catalog, with
answers checked against your live data.

{{< code-card filename="terminal" language="bash" >}}
deeporg eval create --yes
deeporg eval run --yes
{{< /code-card >}}

Prompts, answers, rows, queries and credentials stay in the local
`.graphjin-evals/` store.

{{< benchmark-honesty benchmark="deeporg" >}}
