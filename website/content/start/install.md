---
title: "Install"
description: "Install the DeepORG CLI, and scaffold an API service project if you need one."
nav_group: "start"
doc_kind: "guide"
weight: 10
---

## Install the CLI

Use the install script when you want the native CLI binary:

```bash
curl -fsSL https://deeporg.io/install.sh | bash
deeporg version
```

The command is `deeporg`. The old name, `graphjin`, still works as an alias.

The website serves the script at `/install.sh`; the build syncs it from the repository root before publishing.

Other common development paths:

```bash
deeporg serve --demo
brew install dosco/deeporg/deeporg
```

`deeporg serve --demo` is the fastest smoke test - with no `--path` it extracts the built-in SaaS ops demo (SQLite, no Docker) to `./graphjin-demo` and boots it: schema, seeded data, saved queries, workflows, and (with a model key in `./.env`) the built-in agent. The other [demo verticals](/start/demos/) run from a repo clone via `--path examples/<name>`.

## Scaffold a project

```bash
deeporg serve new my-api
cd my-api
deeporg serve
```

This makes an API service project: GraphQL and REST over your database, with no agent and no MCP server. `dev.yml` and `prod.yml` both set `mode: api` and declare the database under `sources:`.

- With `dev.yml`, a query you run is saved under its name in the `queries` folder, and saved again when you change it.
- With `prod.yml`, only those saved queries are served, exactly as they were saved.

The scaffold also writes `agentic.yml`, a config-file version of the agent server. Most people do not need it: running `deeporg` with no config starts the agent server and sets it up in the browser, as the [quick start](/start/quick-start/) shows.

{{< verified by="TestCmdNewWritesAgenticAndSourcesTemplates" file="cmd/cmd_new_test.go" line="14" >}}
{{< verified by="TestTemplatesDecodeAsConfig" file="cmd/cmd_new_test.go" line="111" >}}

## Connect an AI client

The shortest path needs nothing running — no server, no Docker, no config file, no model key:

```bash
claude mcp add graphjin -- deeporg mcp --demo
codex mcp add graphjin -- deeporg mcp --demo
```

`deeporg mcp --demo` extracts the built-in SaaS ops demo to `./graphjin-demo` and serves MCP over stdio. Your IDE's own model does the reasoning.

To connect to a DeepORG you are already running:

```bash
deeporg mcp add codex
deeporg mcp add claude
```

Start the server first — this path is HTTP-only and sends a real MCP `initialize` probe, so it stops if nothing answers. Defaults are client `codex`, server `http://localhost:8080`, and project scope.

For hosted DeepORG, point the MCP client at the HTTP endpoint:

```bash
codex mcp add graphjin --url https://graphjin.example.com/api/v1/mcp
```

See [MCP](/agentic/mcp/) and [OAuth](/agentic/oauth/) for hosted identity flows.
