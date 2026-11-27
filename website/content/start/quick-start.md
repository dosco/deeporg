---
title: "Quick Start"
description: "Install DeepORG, run one command, and ask your first question about your own data."
nav_group: "start"
doc_kind: "guide"
weight: 20
---

This page takes you from nothing to a first answer from your own data. It takes about five minutes, and you do not write a config file. To see a picture of each screen, read [Set Up And Deploy](/start/deploy/).

## 1. Install and start

[Install the CLI](/start/install/), then run it:

```bash
curl -fsSL https://deeporg.io/install.sh | bash
deeporg
```

DeepORG starts and opens a setup page in your browser. If it cannot open a browser, it prints the link for you to open.

It keeps its own files in `~/.deeporg/`: a small start config and a database for its own data. Nothing is written to the directory you ran it in. If port 8080 is already in use on your computer, DeepORG uses the next free port and tells you which.

## 2. Create your admin account

The first screen asks for an email and a password. This is the account you sign in with, and the one that can change settings later.

## 3. Connect your data

Paste your database's connection URL, such as `postgres://user:password@host:5432/database`, or fill in the host, user and database name separately. DeepORG tests the connection before it saves it, and then tells you how many tables it found. If the connection fails, the page says why and what to change.

If you have no database to hand, choose **Use sample data**. It connects a small sample company: accounts, users, subscriptions, invoices, payments, support tickets and usage events.

By default everyone who signs in can read every table in the databases you connect, and nobody can write to them. You limit this later with [roles and filters](/configure/auth-rbac/).

## 4. Choose a model

The built-in agent uses a model from a provider you have an account with: OpenAI, Anthropic or Google Gemini. Paste an API key. DeepORG checks it with one small request and keeps it encrypted.

You can leave this for later. Until you choose a model, the built-in agent is off.

## 5. Ask your first question

Setup ends on the agent page, with a few questions suggested from your own tables, such as "How many accounts are there?". Pick one, or type your own.

## Connect Claude Code

You can also ask from Claude Code. Open **Connect** in the console and copy the command it shows:

```bash
claude mcp add --transport http --scope user deeporg http://127.0.0.1:8080/api/v1/mcp
```

Then start Claude Code, type `/mcp`, choose `deeporg` and then Authenticate. Your browser opens on DeepORG, where you choose **Allow**. Claude Code signs in as you, so there is no token to copy.

Claude Code gets its answers by asking DeepORG's built-in agent, so choose a model first.

## Invite your team

Open **Admin → Setup**, enter a teammate's email, and send them the link it makes. They choose their own password. The same page is where you connect another database or change the model.

## Run it on a server

Set the address to listen on, then open the setup link DeepORG prints. The link is written for the server itself, so replace the host with the server's address if you open it from another computer.

```bash
DEEPORG_LISTEN=0.0.0.0:8080 deeporg
```

There is one limit to know about: DeepORG keeps its own data in a local file for now, so the server needs a disk that survives restarts.

[Set Up And Deploy](/start/deploy/#part-2-put-it-on-a-server) goes through a server step by step: Docker, HTTPS, inviting your team and what to back up.

## Using config files instead

Before this setup flow, DeepORG was configured with YAML files, and that still works. If the directory you start it in has a `config/` folder, DeepORG uses those files and shows no setup page. This is the path to use when you run DeepORG as an API service for an application, with [saved queries](/start/saved-queries/) and no agent. Every config file needs a `sources:` block; `deeporg serve new my-api` writes a project with one.

```yaml
# config/dev.yml
mode: api
sources:
  - name: app
    kind: database
    type: postgres
    default: true
    connection_string: postgres://user:password@localhost:5432/app
    schema: public
```

```bash
deeporg serve
```

The rest of this page uses that API: the GraphQL endpoint every DeepORG server has.

## First query

DeepORG discovers tables and relationships, then compiles GraphQL into database work.

```graphql
query {
  products(limit: 3, order_by: { id: asc }) {
    id
    name
    owner {
      id
      fullName: full_name
    }
  }
}
```

The response shape follows the GraphQL selection. There are no user-written resolvers for database fields.

{{< verified by="Example_query" file="tests/query_test.go" line="18" >}}

## Call the endpoint

Keep a raw endpoint call in your smoke test even if you used the Workbench. This verifies the HTTP API path your application or integration will call.

```bash
curl http://localhost:8080/api/v1/graphql \
  -H 'content-type: application/json' \
  -d '{"query":"query { products(limit: 3, order_by: { id: asc }) { id name } }"}'
```

Pass variables as JSON. Keep filters inside the GraphQL query shape and pass leaf values through variables:

```graphql
query Products($maxPrice: Float!) {
  products(where: { price: { lteq: $maxPrice } }, order_by: { price: asc }) {
    id
    name
    price
  }
}
```

```json
{ "maxPrice": 25 }
```

## Next steps

- Let DeepORG run the loop for you: the [built-in agent](/agentic/server-agent/) turns one instruction into a typed, evidence-backed answer.
- Learn the [query language](/core/query-language/).
- Add [filters](/core/filters/) and [cursor pagination](/core/ordering-cursors/).
- Understand the config file and every way to change it in [How Configuration Works](/configure/how-it-works/).
- Lock down production with [saved queries](/start/saved-queries/) and [RBAC](/configure/auth-rbac/).
