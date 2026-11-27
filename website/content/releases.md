---
title: "Release notes"
nav_title: "Release notes"
description: "What changed in each DeepORG release, and what to do when you upgrade."
---

This page says what changed in each release and what you need to do when you upgrade. The downloads for every version are on the [GitHub releases page](https://github.com/dosco/deeporg/releases).

## Version 3.25

These notes cover the releases from 3.23.1 to 3.25.0.

This release is about running DeepORG on a server for a team. An admin can now look after people's accounts and choose who reads each table, and there is a way back in when a password is lost. If you upgrade, there is nothing you have to change.

### Accounts

- An admin can change what a person may do, from **Admin → Setup**. The choice is between using DeepORG and being an admin, and it takes effect at once.
- An admin can give a person a link for choosing a new password. DeepORG makes the link and the admin sends it. It works once, for 24 hours. Choosing the new password signs that account out everywhere else, including in any agent it connected.
- An admin can remove a person. They are signed out at once.
- An admin cannot remove their own account or take their own admin rights away, and nobody can remove the last admin.
- `deeporg reset-password <email>` makes a password link from the command line, on the machine DeepORG runs on. It is the way back in for an admin who has lost their password. The server keeps running while you use it.
- The sign-in screen has a "Forgot your password?" line that says both of these.

### Who can read each table

- An admin can choose who reads each table of a connected database, from **Admin → Setup**: everyone who signs in, admins only, or nobody. Before, this needed a config file.
- The agent only uses tables that are open to everyone. It is not told that the others exist.
- Connecting a database again under the same name keeps these choices.

### Running on a server

- A new server passes its health check before setup is finished. `/health` used to answer with an error until a database was connected, so hosting that restarts a failing program could stop a new server before anyone had set it up.
- The Docker image listens for other computers on port 8080 without being told to. Publishing the port is enough.
- When DeepORG cannot write to its folder, it says which folder and what to mount.
- Wrong sign-in guesses are counted against the person who made them when DeepORG sits behind a web server that passes requests on. Before, they were all counted against the web server, so a few wrong guesses by one person stopped everyone from signing in for a minute.
- [Set Up And Deploy](/start/deploy/) is a new page that shows each setup screen and goes through a server step by step.

### Connecting an AI client

- `deeporg mcp add` now adds the server under the name `deeporg`, which is the name the console's Connect page uses. If an earlier version added it as `GraphJin` or `graphjin`, adding it again replaces that entry.
- The catalog no longer tells an agent that the security report can be read on a server that does not serve it.
- The console's Runtime page no longer shows a failed security query on a new server.

### What DeepORG does not do yet

- A database can be added or replaced from the console, but not disconnected.
- The console hides whole tables. It has no screen for hiding some rows of a table, or for giving one person more than another.
- A server set up in the browser keeps its own data in a local file, so it needs a disk that survives restarts, and you run one copy.
- An agent that hands its sign-in back through an address of its own kind, as some editors do, is refused. Agents that listen on your own computer or on an `https` address work.

## Version 3.23

Released on 26 November 2026. These notes cover every release from 3.21.27 to 3.23.0.

This release changes how you start DeepORG. You run one command and finish setup in your browser, with no config file to write. It also adds accounts, invites and a way to connect Claude Code without copying a token. If you run DeepORG from config files, read [what to change when you upgrade](#what-to-change-when-you-upgrade) before you install it.

### Setup in the browser

- Running `deeporg` with nothing after it starts the server and opens a setup page in your browser.
- Setup has three steps. You create an admin account, connect a database, and choose a model for the built-in agent.
- If you have no database to hand, the second step offers sample data: a small company with accounts, users, subscriptions, invoices, payments, support tickets and usage events.
- Each step checks what you give it. A database connection is tested, and the page tells you how many tables it found. A model key is checked with one small request to the provider. When something fails, the page says why and what to change.
- DeepORG keeps its own files in `~/.deeporg/`: a small start config and a database for its own data. Database passwords and the model key are stored encrypted.
- The terminal now prints the setup link and nothing else. If port 8080 is taken on your computer, DeepORG uses the next free port and tells you which.
- After setup, the agent page suggests questions about your own tables, such as "How many accounts are there?".

The [quick start](/start/quick-start/) walks through each step.

### Accounts and invites

- People sign in with an email and a password. Before this release, someone had to make a sign-in token by hand.
- An admin invites a teammate from **Admin → Setup**. DeepORG makes a link for the admin to send, and the teammate chooses their own password. A link works once, for one week. DeepORG does not send the email for you.
- **Admin → Setup** is also where an admin connects another database or changes the model later.
- On a server set up this way, everyone who signs in can read every table in the connected databases, and nobody can write to them. An admin narrows this with [roles and filters](/configure/auth-rbac/).

### Connecting Claude Code

- The console has a **Connect** page with one command to copy. The command holds the server's address and no secret.
- When Claude Code connects, your browser opens on DeepORG and asks whether to allow it. Claude Code is then signed in as you.
- The token Claude Code receives works only for the agent connection. It cannot be used on the other parts of the server.
- Claude Code gets its answers by asking DeepORG's built-in agent, so choose a model during setup first.
- `deeporg mcp add claude <url>` now sets up Claude Code. In earlier versions it set up Codex.
- Claude Code can now see DeepORG's tools on an agent server. Earlier versions described one tool's output in a way that strict clients refused, and the client then showed no tools at all.

We tested this with the client library that Claude Code is built on. If your version of Claude Code behaves differently, please [open an issue](https://github.com/dosco/deeporg/issues).

### The API service

DeepORG can still run the way it did before agents: as an API over your database that serves saved queries. This is now called the API service, and you choose it with `mode: api`. The older name, `mode: prod`, means the same thing.

- The API service has no agent, no catalog and no MCP server, in development or in production.
- While `production` is `false`, a query you run is saved under its name, and saved again when you change it. When `production` is `true`, only the saved queries are served, and a client cannot change one.
- The API service limits data with the rules you write for each role under `roles`, as it always has.
- `deeporg serve new my-api` now makes an API service project. Starting DeepORG from inside that folder uses the project's own files.
- The console of an API service opens on the Workbench, where you try queries. It shows the Workbench and the API reference and nothing else.

### What to change when you upgrade

Go through this list if you already run DeepORG from config files. A new install needs none of it.

**Every config file needs a `sources:` block.** A database is now declared as a source. A config file that still has the older `database:` section is refused when DeepORG starts, and the error prints the block to add.

```yaml
# Before
database:
  type: postgres
  host: localhost
  dbname: shop

# After
sources:
  - name: default
    kind: database
    default: true
    type: postgres
    host: localhost
    dbname: shop
```

**The API service no longer writes access rules for you.** In earlier versions, a config with `sources:` in `mode: prod` limited each role to the rows of its own account, using an `account_id` column. That no longer happens. If you relied on it, write those limits as rules under `roles` before you upgrade. Nothing warns you about this one, so check it first.

```yaml
roles:
  - name: user
    tables:
      - name: orders
        query:
          filters: ["{ account_id: { eq: $account_id } }"]
```

**An `access:` block on a database source is an error in the API service.** That block belongs to the agent server. Move its rules under `roles`.

**The agent server lets every signed-in person read every table.** In earlier versions the agent server limited each person to the rows of their own account by default, and blocked tables that had no `account_id` column. If you want that back, set it on the source:

```yaml
sources:
  - name: app
    kind: database
    access:
      read: account
```

**Agents must sign in.** On an agent server with accounts, a client that connects without signing in is refused and told where to sign in. In earlier versions it connected and every tool then refused it.

**The catalog needs sign-in, and only an admin can read the running configuration.** This applies to the agent server.

**`production: true` does nothing on the agent server.** It only has a meaning in the API service.

**The MCP server cannot be turned on in the API service.** Setting `mcp.disable: false` in a production config used to turn it on. It no longer does.

**`deeporg serve db store init` and `deeporg serve db store status` are removed**, along with the `artifacts.auto_init` setting. DeepORG now creates the tables for its own data each time it starts. If your config says `auto_init: false`, DeepORG refuses to start and tells you what to do: allow its database user to create tables, and delete the setting.

### What version 3.23 did not do

The list at the [top of this page](#what-deeporg-does-not-do-yet) is the current one.

- An admin cannot remove a person, change their role or reset a password from the console.
- A database can be added or replaced from the console, but not disconnected.
- A server set up in the browser keeps its own data in a local file, so it needs a disk that survives restarts.
- An agent that hands its sign-in back through an address of its own kind, as some editors do, is refused. Agents that listen on your own computer or on an `https` address work.
