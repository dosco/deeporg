---
title: "Answers with the data behind them"
headline: "Every answer shows the data behind it. A made-up one is blocked."
description: "An answer from DeepORG comes with the data it is based on and a record of what actually ran. Guards block a claim that has nothing behind it, the database does the arithmetic, and a refusal says what would unblock it."
lede: "An answer from DeepORG comes with the data it is based on and a record of what actually ran. An answer that claims something with nothing behind it is blocked, however confident it sounds."
scene: "ledger"
scene_label: "Animated diagram: a claim with no query behind it is blocked, and a claim backed by three queries and their rows is answered with evidence attached."
scene_caption: "checked before an answer leaves"
actions:
  - label: "Try it in 2 minutes"
    href: "/#quickstart"
  - label: "See it graded in public"
    href: "/benchmark/"
landing: true
toc: false
---

{{< band label="Answers backed by evidence" title="If no query ran, the answer is blocked." >}}

{{< grid >}}

- **01. The model answers anyway.** It names roast batches to hold.
- **02. No query ran.** DeepORG keeps a record of every query, and nothing in it backs the claim.
- **03. DeepORG blocks it.** The made-up answer never reaches the person who asked.

{{< endgrid >}}

What the caller gets instead is a refusal it can act on:

{{< code-card filename="response.json" language="json" >}}
{
  "status": "blocked",
  "refusal": {
    "code": "ungrounded_answer_fields",
    "blocked_action": "answer",
    "unblock": [
      {
        "tool": "query_catalog"
      }
    ],
    "retryable": true
  }
}
{{< /code-card >}}

{{< endband >}}

{{< band label="What comes back" title="The answer, its data, and how it was reached." >}}

A reply is more than a sentence. Next to the `answer` it carries the `data` behind it and the `evidence`: a record of what the agent looked up, checked and ran to get there.

It also says how the result is best shown: one value, a bar chart, a line or a table, with the rows themselves. A chat window can draw the chart without guessing which column is the label.

{{< endband >}}

{{< band label="Who does the arithmetic" title="The model plans. The database computes." >}}

The common way an agent gets a confident wrong number is to fetch a page of rows and add them up in its head. The page was the first ten of four hundred, and the sum looks fine.

DeepORG does not let the model do sums. The lead agent never computes an aggregate itself. It asks, and the database computes the count, the total or the ranking where the data is.

We test this in public. On [the public benchmark](/benchmark/) a full pass needs the right answer and the right way of getting it: a correct number reached by adding up a page of rows does not count.

{{< endband >}}

{{< band label="Guards" title="The model cannot skip these checks." >}}

Guards are checks that run between the model and the person who asked. If the model skipped a required step, its answer is changed to `blocked` and the reply says why. Only what actually ran counts.

{{< grid >}}

- **Named things must have been seen.** In a conversation, an identifier that no question returned gets one chance to be restated, then an `ungrounded_answer_fields` refusal.
- **A held change is never reported as done.** When a change is waiting for approval, the reply is DeepORG's own sentence, not the model's.
- **Memory is not evidence.** A remembered preference shapes how it answers. It is never the source of a figure.
- **A refusal is specific.** A stable code, the action that was blocked, the reasons, the steps that would unblock it, and whether trying again can help.

{{< endgrid >}}

{{< endband >}}

{{< band label="When it is wrong" title="Anyone can mark an answer wrong." >}}

Anyone can mark an answer helpful or wrong and say what the right answer is. A correction is kept with the queries the agent ran, so anyone can run them again and see what went wrong.

Answers nobody objected to are useful too. Every question answered by one database-computed figure can become a test whose answer key is the query itself, re-run whenever the test runs. An answer a person marked wrong never becomes a test. [Learning from real work](/experience/) shows how.

{{< endband >}}

{{< band label="In the docs" title="Go deeper." >}}

- [How the agent works](/agentic/server-agent/): replies, guards and refusals.
- [Conversations](/agentic/lead-agent/): how the lead keeps its answers grounded.
- [Benchmark methodology](/benchmarks/deeporg/methodology/): how answer, method, behavior and safety are scored.
- [Evaluating the agent](/agentic/evaluation/) on your own questions.
- [DeepORG vs text-to-SQL](/compare/deeporg-vs-text-to-sql/): why the engine writes the SQL, not the model.

{{< endband >}}
