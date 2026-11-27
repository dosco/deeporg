---
title: "Install"
description: "Install the DeepORG command, start it, and connect an AI client."
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

With Homebrew:

```bash
brew install dosco/deeporg/deeporg
```

The website serves the script at `/install.sh`; the build syncs it from the repository root before publishing.

## Start it

```bash
deeporg
```

DeepORG starts and opens a setup page in your browser. You create an admin account, connect a database or choose the sample data, and choose a model. There is no config file to write. The [quick start](/start/quick-start/) goes through the steps, and [Set Up And Deploy](/start/deploy/) shows each screen and how to put it on a server.

If you forget your password, ask an admin for a new password link. If you are the only admin, make the link yourself on the machine DeepORG runs on:

```bash
deeporg reset-password you@example.com
```

{{< verified by="TestAdminChangesRolesResetsPasswordsAndRemovesPeople" file="cmd/agentic_people_test.go" line="25" >}}

## Try a demo

```bash
deeporg serve --demo
```

With no `--path`, this extracts the built-in SaaS ops demo (SQLite, no Docker) to `./graphjin-demo` and boots it: schema, seeded data, saved queries, workflows, and (with a model key in `./.env`) the built-in agent. The other [demos](/start/demos/) run from a copy of the repository with `--path examples/<name>`.

## Connect an AI client

**To a DeepORG you are running.** Sign in to the console, open **Connect**, and copy the command it shows. For Claude Code it looks like this, with your server's address:

```bash
claude mcp add --transport http --scope user deeporg http://127.0.0.1:8080/api/v1/mcp
```

Claude Code then signs in as you in your browser, so there is no token to copy. `deeporg mcp add claude <url>` does the same from the terminal, and `deeporg mcp add codex <url>` sets up Codex. Either way the server is added under the name `deeporg`. If an earlier version added it as `GraphJin` or `graphjin`, adding it again replaces that entry.

Start the server first. `deeporg mcp add` sends a real MCP `initialize` request and stops if nothing answers. With no address it uses `http://localhost:8080`.

**With nothing running.** This needs no server, no Docker, no config file and no model key:

```bash
claude mcp add deeporg -- deeporg mcp --demo
codex mcp add deeporg -- deeporg mcp --demo
```

`deeporg mcp --demo` extracts the built-in SaaS ops demo to `./graphjin-demo` and serves MCP over stdio. Your own AI client's model does the reasoning.

See [MCP](/agentic/mcp/) and [OAuth](/agentic/oauth/) for other ways to sign clients in.

## Scaffold an API service project

DeepORG can also run as an API over your database, with no agent. That path uses config files.

```bash
deeporg serve new my-api
cd my-api
deeporg serve
```

This makes an API service project: GraphQL and REST over your database, with no agent and no MCP server. `dev.yml` and `prod.yml` both set `mode: api` and declare the database under `sources:`.

- With `dev.yml`, a query you run is saved under its name in the `queries` folder, and saved again when you change it.
- With `prod.yml`, only those saved queries are served, exactly as they were saved.

The scaffold also writes `agentic.yml`, a config-file version of the agent server. Most people do not need it, because running `deeporg` with no config starts the agent server and sets it up in the browser.

{{< verified by="TestCmdNewWritesAgenticAndSourcesTemplates" file="cmd/cmd_new_test.go" line="14" >}}
{{< verified by="TestTemplatesDecodeAsConfig" file="cmd/cmd_new_test.go" line="111" >}}
