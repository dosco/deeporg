---
title: "Roll out DeepORG in your company"
nav_title: "Roll it out"
type: guides
headline: "What a rollout takes, step by step."
description: "A step-by-step guide for whoever rolls out an AI agent across a company's data: connect the systems, set who may see what, write down what the company's words mean, check answers until people trust them, hand it over, give it recurring work, and know whether it is working."
lede: "Choosing the model is the small part of an AI rollout. Most of the work is connecting the systems, deciding who may see what, writing down what the company's words mean, checking answers until people trust them, and then giving the agent real work to do every week. Companies usually bring in forward-deployed engineers or consultants for it. This page is for whoever does that work. For each step, it shows the part of DeepORG that handles it."
actions:
  - label: "Try a demo first"
    href: "/#demos"
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

- **It finds your tables for you.** DeepORG builds a searchable map of every table, column and relationship it finds. The agent looks things up in the map and writes its own queries.
- **It reads relationships from your foreign keys.** Ones the schema never declared can be added in config.
- **One answer can use several systems.** It can use the app database, the warehouse and an HTTP API together.
- **A read-only source cannot be changed.** `read_only: true` blocks every write to that source, whatever a role or the agent asks for.

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
- **Changes wait for a person.** A change the agent proposes is first tried out in a transaction that is rolled back, then held until someone approves that exact version.
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

- **The agent can suggest notes.** It can draft one from a conversation, but it cannot approve its own draft. A person has to approve it.
- **A note only adds context.** It cannot grant access or make a read-only source writable.
- **A note is kept when its table goes away.** If the table a note points at disappears, the note is kept and marked stale.

{{< endgrid >}}

[How notes work](/agentic/annotations/)

{{< endband >}}

{{< band label="Step 4 · Trust" title="Check the answers until people trust them." >}}

People trust answers they have checked, so DeepORG makes each answer quick to check.

{{< grid >}}

- **Every answer shows its queries.** People can see what was asked of which system, and the agent's claims are backed by what those queries returned.
- **The database does the maths.** Sums, counts and rankings run in your database, so the model does not add up rows itself.
- **People can say when it is wrong.** In the console, each answer has a Helpful and a Wrong button. Wrong asks what it should have said, and the report is kept with the queries the agent ran.
- **Answers become tests.** A question answered with one figure the database computed can become a test. The test asks the agent again and checks its answer against what the same query returns today. Run the tests before you switch to a new model.

{{< endgrid >}}

Read the corrections, then turn real questions into tests:

```bash
# Answers marked wrong, with the corrections (admin)
curl -H "Authorization: Bearer $TOKEN" \
  "$DEEPORG_URL/api/v1/agent/feedback?format=corpus"

# Turn real questions into tests checked against the database
deeporg eval mine --url $DEEPORG_URL --token $TOKEN
deeporg eval run --suite eval/mined.yml
```

[Answers with the data behind them](/why/answers-with-evidence/) · [Learning from real work](/experience/)

{{< endband >}}

{{< band label="Step 5 · Hand it over" title="Hand it to the people who ask." >}}

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

- **People can ask in the console.** They ask in plain words at `/agent` and see the queries behind each answer.
- **They can use their own AI client.** `deeporg mcp add claude <url>` or `deeporg mcp add codex <url>` connects Claude or Codex to the same server, under the same rules.
- **What needs a person goes to an inbox.** A change waiting for approval, a routine's answer, a run that stopped: each lands in the console's Inbox and at `/api/v1/notifications`, with a live stream beside it.

{{< endgrid >}}

Most people outside engineering work in Slack, Teams or email, and DeepORG has no app for those yet. What it has today is a webhook: a watch can post each event as JSON to an address you allow in `watches.webhook_allow`, and a workflow tool such as Power Automate or Zapier can turn that into a message in the channel where the team already talks.

[Agent docs](/agentic/server-agent/) · [Watches and their delivery](/agentic/watches/)

{{< endband >}}

{{< band id="recurring" label="Step 6 · Recurring work" title="Give it a recurring job." >}}

People come to trust the agent by asking it questions. The work that makes a rollout worth paying for is the job someone does every week: chase tickets nobody answered, flag accounts that stopped using the product, check that last night's load arrived. A routine is that job, written down once. It is a watch that, each time it runs, gives the agent its prompt to work on:

```graphql
mutation {
  gj_watch(insert: {
    name: "stale_tickets_weekday"
    schedule: { cron: "0 9 * * 1-5", timezone: "America/New_York" }
    delivery_json: {
      kind: "agent"
      prompt: "Which open support tickets have had no reply for three days? Raise each one's priority to high."
    }
  }) { id next_fire_at status }
}
```

{{< grid >}}

- **It looks before it proposes.** When the routine runs, on its schedule or when the data it watches changes, the agent looks up what the decision depends on, then says in plain words whether a change is called for.
- **A change waits for its owner.** Like any change the agent proposes, it is tried out in a transaction that is rolled back, then held. The owner gets an "action needed" notification and approves or rejects that exact version.
- **It checks its own work.** A change can say what it should achieve: a query and the number it should reach. DeepORG measures it before the change and again after, and records `hit` or `miss`. On a miss, the agent is asked to look into why.
- **It has a budget.** `agent.budgets` caps model tokens per person and per routine over a rolling day, on by default in agentic mode. A run over its cap stops, and the owner is told.

{{< endgrid >}}

A routine proposes changes only with write approvals on, the default in agentic mode. Without them it answers and changes nothing.

[What a question left running can do](/watch/) · [How routines work](/agentic/watches/#routines-that-act)

{{< endband >}}

{{< band id="working" label="Step 7 · Measure" title="Know whether it is working." >}}

Three questions tell you whether a rollout is landing, and DeepORG keeps a record of each.

{{< grid >}}

- **Do people trust the answers?** Every Helpful and Wrong rating is kept, with the queries behind the answer and, for a wrong one, what it should have said. `GET /api/v1/agent/feedback` lists them; an admin sees everyone's.
- **Are the answers still right?** The tests made from real questions in Step 4 pass or fail against today's data. Run them every week, and before you change models.
- **Do the changes work?** `GET /api/v1/agent/reliability` tallies how each routine's and each saved agent's changes turned out, hit or miss.

{{< endgrid >}}

There is no single adoption report yet that puts these together with who asked and how often. For now, these three are the record.

```bash
# How the agent's changes turned out, per routine and kind of change
curl -H "Authorization: Bearer $TOKEN" "$DEEPORG_URL/api/v1/agent/reliability"
```

[Learning from real work](/experience/)

{{< endband >}}

{{< band id="cost" label="Cost" title="Keep the model bill down." >}}

You pay your model provider for the tokens the agent uses. Two things keep that bill down.

{{< grid >}}

- **The database does the counting.** A total over a million rows reaches the model as one number. So the size of your tables does not drive the cost.
- **You can try a cheaper model on your own questions.** The tests from Step 4 are your company's real questions, checked against your database. Run them on a cheaper model, and switch if it passes the same tests.

{{< endgrid >}}

{{< endband >}}
