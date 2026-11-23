---
title: "Roll out DeepORG in your company"
nav_title: "Roll it out"
type: guides
headline: "Most of an AI rollout is not the model."
description: "A step-by-step guide for whoever rolls out an AI agent across a company's data: connect the systems, set who may see what, write down what the company's words mean, check answers until people trust them, and put the agent where people already work."
lede: "Most of the work is connecting the systems, deciding who may see what, writing down what the company's words mean, and checking answers until people trust them. Companies usually bring in forward-deployed engineers or consultants for it. This page is for whoever does that work. It goes step by step and shows the part of DeepORG that handles each one."
actions:
  - label: "Try the demo first"
    href: "/#quickstart"
  - label: "Install"
    href: "/start/install/"
landing: true
toc: false
---

{{< band label="Before you start" title="Pick five questions people already ask." >}}

Start with questions a team asks every week, and whose answers someone already knows. *How many support tickets are open? Which accounts renew this month? What did we invoice last quarter?*

A known answer makes the first check cheap. When DeepORG gets one of these right, the person who knows the number can see it at a glance. When it gets one wrong, you find out on day one, not in a meeting a month later.

{{< endband >}}

{{< band label="Step 1 · Connect" title="Point it at what the company already runs." >}}

Name each system in the config, read-only to begin with:

```yaml
sources:
  - name: app
    kind: database
    type: postgres
    connection_string: ${DATABASE_URL}
    read_only: true
  - name: warehouse
    kind: database
    type: snowflake
    connection_string: ${SNOWFLAKE_DSN}
    read_only: true
```

{{< grid >}}

- **No tool for each question.** DeepORG builds a searchable map of every table, column and relationship it finds. The agent looks things up in the map and writes its own queries, so nobody has to build a tool per question.
- **Joins come from your keys.** Relationships are read from your foreign keys. Ones the schema never declared can be added in config.
- **Several systems, one question.** One answer can draw on the app database, the warehouse and an HTTP API together.
- **Read-only means read-only.** `read_only: true` blocks every write to that source, whatever a role or the agent asks for.

{{< endgrid >}}

[Setup for each database](/guides/) · [Every kind of system it connects to](/why/every-system/)

{{< endband >}}

{{< band label="Step 2 · Access" title="Decide who may see what, once." >}}

Write the rules once. Every query the agent writes is checked against them before it runs, just like a query from any other client:

```yaml
roles:
  - name: user
    tables:
      - name: invoices
        query:
          filters:
            - "{ account_id: { eq: $account_id } }"
          columns: ["id", "amount", "due_date", "status"]
```

{{< grid >}}

- **Identity comes from the sign-in token.** It never comes from anything the person or the model types.
- **Hidden columns stay hidden in totals too.** A role that cannot read a column cannot sum it either.
- **Changes wait for a person.** A change the agent proposes is rehearsed in a transaction that is rolled back, then held until someone approves that exact version.
- **One switch makes the agent read-only.** `agent.read_only` limits it to reading, whatever a role allows.

{{< endgrid >}}

[Your rules, your approval](/why/your-rules/)

{{< endband >}}

{{< band id="terms" label="Step 3 · Your words" title="Write down what your terms mean." >}}

Every company has words that mean something specific: *active customer*, *at risk*, *shipped late*. Write each one down once, as a note on the table or column it belongs to:

```graphql
mutation {
  gj_artifacts(insert: {
    kind: "annotation"
    target_ref: "table:app.public.accounts"
    content: "Active = a paid invoice in the last 90 days."
  }) { id tier }
}
```

A new note starts as a draft that only its author can see. After someone approves it, the agent reads it whenever it opens that table.

{{< grid >}}

- **The agent can suggest notes.** It can draft one from a conversation. It cannot approve its own draft: a person has to, in a later step.
- **A note is context, not a permission.** It cannot grant access or make a read-only source writable.
- **Notes go stale safely.** If the table a note points at disappears, the note is kept and marked stale.

{{< endgrid >}}

[How notes work](/agentic/annotations/)

{{< endband >}}

{{< band label="Step 4 · Trust" title="Check the answers until people trust them." >}}

Trust comes from checking, and DeepORG makes each answer quick to check.

{{< grid >}}

- **Every answer shows its queries.** People can see what was asked of which system, and the agent's claims are backed by what those queries returned.
- **The database does the maths.** Sums, counts and rankings run in your database, not in the model's head over a page of rows.
- **People can say when it is wrong.** In the console, each answer has a Helpful and a Wrong button. Wrong asks what it should have said, and the report is kept with the queries the agent ran.
- **Answers become tests.** A question answered with one figure the database computed can become a test whose answer key is the query itself. Run the tests again before you switch to a new model.

{{< endgrid >}}

Read the corrections, then turn real questions into tests:

```bash
# Answers marked wrong, with the corrections (admin)
curl -H "Authorization: Bearer $TOKEN" \
  "$DEEPORG_URL/api/v1/agent/feedback?format=corpus"

# Real questions, turned into tests the database marks
deeporg eval mine --url $DEEPORG_URL --token $TOKEN
deeporg eval run --suite eval/mined.yml
```

[Answers with the data behind them](/why/answers-with-evidence/) · [Learning from real work](/experience/)

{{< endband >}}

{{< band label="Step 5 · Hand it over" title="Put it where people already work." >}}

Run DeepORG in agentic mode. It starts from your production config and adds the agent:

```yaml
# config/agentic.yml
inherits: prod
mode: agentic
```

```bash
GO_ENV=agentic deeporg serve --path ./config
```

{{< grid >}}

- **A console for everyone.** People ask in plain words at `/agent` and see the queries behind each answer.
- **Their own AI client.** `deeporg mcp add claude <url>` or `deeporg mcp add codex <url>` connects Claude or Codex to the same server, under the same rules.
- **Questions left running.** A question can keep running against live data. DeepORG speaks up when the answer changes, or when something expected never arrives.

{{< endgrid >}}

[What a question left running can do](/watch/) · [Agent docs](/agentic/server-agent/)

{{< endband >}}
