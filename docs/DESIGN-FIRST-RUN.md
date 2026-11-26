# First-Run Setup Design

Status: proposal, not implemented. Written 2026-11-24.

## The goal

A newcomer installs DeepORG, runs one command, follows the page in front of
them, and gets a real answer from their own data in about five minutes. They do
not open a config file, pick a mode, or read the docs first.

Everything in this document serves that. Where a design choice makes the
newcomer's path longer or asks them to understand something, it is the wrong
choice.

## Why this is needed

On 2026-11-24 we started a fresh install the way a new user would: an empty
directory, the current binary, a Postgres database. It failed at every step.
These were seen by running the server, not by reading code:

| Step | What happened |
| :--- | :--- |
| Start with no config | The server writes three YAML files pointing at a host called `db` and exits. |
| Use `connection_string: ${DATABASE_URL}` as the docs show | The server crashes. The value is never expanded and `initPostgres` (`serv/db.go`) ignores the parse error. |
| Start the generated `agentic.yml` | It has no `auth:` block, so MCP, GraphQL and the console all answer 401. The console says "No access" and offers no way to sign in. |
| Add a JWT secret and sign in as admin | `query_catalog` fails, and the watch, schedule, task and thread loops fail on every cycle, all with "invalid query or fragment name". |
| Remove `production: true` to fix that | Raw queries, mutations, config updates, schema updates and workflows switch on "by default (dev mode)" for any setting the file does not name. |
| Ask a question | Every table without an `account_id` column is blocked for every role, the admin included. |
| Ask the agent to connect a second database | "You don't have permission to change GraphJin configuration." |

These share one cause. An older API-backend flow (`mode: dev`, `mode: prod`) and
the newer agentic flow are layered on each other. `mode`, a separate
`production` flag, eight `mcp.allow_*` switches and the auth type are
independent settings that can disagree. The generated `agentic.yml` is one such
disagreement, and no demo or smoke runs it: the demo agentic configs inherit
`dev.yml` and leave out `production: true`.

Today's quick start opens by telling the reader to write a config file.

## Decisions this design starts from

- DeepORG is an agentic platform first. A new install is agentic.
- There is one way to start: run `deeporg`.
- Sign-in is always required, so setup creates an admin account.
- Setup asks for three things: a database, an admin account, and a model provider with its key.
- One model serves both the top-level agent and the worker at setup. The decision model stays an optional later setting.
- The admin does everything after setup: more databases, teammates, access rules.
- Teammates join by invite. There is no self-signup.
- **Every signed-in user can read every table in the connected databases.** This is a general database agent. The only limits are the ones the admin sets: table blocks, row filters and roles. DeepORG's own storage is never readable. This applies to every agentic install, not only new ones.
- **Agentic keeps nothing on disk except a small start config.** Everything else is in DeepORG's own database, which this document calls the store.
- **On a laptop the store is a local SQLite database** that DeepORG creates, and the start config and the store live in `~/.deeporg/`.
- **The big config is not shown to users.** They see the small start config, and change everything else through setup, the console, the agent or Claude Code.
- **Saved query files and workflow files are not part of agentic.** The agent writes queries as it goes. Reusable pieces such as fragments are kept in the store under the user who made them, which is how it works today.
- `mode: dev` and `mode: prod` are the legacy flow for people who use DeepORG as an API service without the agent. They keep working and get no new features.
- No platform-specific deploy files (Helm charts, fly.toml and the like).

## Part 1: The newcomer's path

### What they do

**1. Install and start**

```bash
curl -fsSL https://deeporg.io/install.sh | bash
deeporg
```

On a laptop the browser opens on the setup page. Nothing else is asked on the
command line. Running `deeporg` with no arguments starts the server; today it
prints help. The container image does the same.

**2. Create your admin account**

One screen: email and password. Nothing else.

**3. Connect your data**

One screen, two choices.

- **Use sample data.** Loads the built-in demo company so someone with no
  database to hand still reaches an answer. This replaces `--demo` as a separate
  way to start.
- **Connect a database.** Paste a connection URL, or fill in type, host, port,
  user, password and database name. The page tests the connection and then says
  what it found: "Connected. Found 42 tables."

**4. Pick a model**

One screen: provider and key. The page makes one live call to check the key and
shows which model it will use. "Skip for now" is allowed; the page says what
will not work until a key is added: the built-in agent and routines. A
connected agent such as Claude Code asks the built-in agent, so it gets no
answers until a key is added either.

**5. Ask your first question**

Setup ends on the agent, with three or four questions suggested from the tables
it found. Below them is the command to connect Claude Code, ready to copy.

### Rules every step follows

- One obvious action per screen, with a default that works.
- Every input is tested before the step is accepted. Nobody finishes setup with a database that does not connect or a key that does not work.
- An error says what went wrong and what to do about it, in plain words. "We could not reach `db.example.com` on port 5432. Check the host name, and that this server is allowed through the database firewall." It never shows a stack trace or an internal name.
- The newcomer never sees a mode, a flag, or a secret they have to generate, and never has to open a file.
- Any step can be redone later from the console.
- The same steps exist as prompts in `deeporg setup` for someone working in a server shell.

### What the quick start will say

The page that describes this flow is designed with it. Draft, in the site's
voice:

> **Quick start**
>
> Install DeepORG and start it.
>
> ```bash
> curl -fsSL https://deeporg.io/install.sh | bash
> deeporg
> ```
>
> Your browser opens a setup page. It asks you for three things.
>
> 1. **An admin account.** Choose an email and a password. You use this account to sign in and to change settings later.
> 2. **Your data.** Paste the connection URL for your database. If you do not have one to hand, choose the sample data and connect your own database later.
> 3. **A model.** Choose a provider and paste its API key. DeepORG checks that the key works.
>
> When setup finishes you are on the agent page. Ask it a question about your data.

## Part 2: The start config

The start config is the only file. It holds what the server needs before it can
reach its own database, and nothing else.

```yaml
# deeporg.yml
store: postgres://deeporg:…@db.internal:5432/deeporg
secret_key: 4f0c…e91a
listen: 0.0.0.0:8080
```

| Key | What it is | Environment variable |
| :--- | :--- | :--- |
| `store` | The database where DeepORG keeps its own data: accounts, settings, threads, routines, the catalog of your tables. | `DEEPORG_STORE` |
| `secret_key` | One key, used to sign sign-in sessions and to encrypt the database passwords and model keys kept in the store. | `DEEPORG_SECRET_KEY` |
| `listen` | Address and port. Defaults to `0.0.0.0:8080`; `PORT` is honoured. | `DEEPORG_LISTEN` |
| `log_level` | Optional. Needed before the store is open, so it lives here. | `DEEPORG_LOG_LEVEL` |

Every key can be given as an environment variable instead, so a platform that
offers no files at all still works.

**On a laptop** the newcomer never writes this file. The first run creates
`~/.deeporg/deeporg.yml` with a generated secret key and a `store` that points
at a local SQLite database beside it, so nothing has to be installed and
`deeporg` behaves the same from any directory. The store is still a database;
it is one file that DeepORG owns, with nothing loose beside it.

The server looks for the start config in this order: the file named by
`--config`, then `deeporg.yml` in the current directory, then
`~/.deeporg/deeporg.yml`.

**Which databases can hold the store.** DeepORG reads and writes its store
through its own compiler, so the store is not tied to one kind of database.
What limits it today is creating the tables, which is written out per database
for SQLite, Postgres and MySQL (`ensureStoreTables`, `serv/artifacts.go`).
Those three are what the first version supports: SQLite on a laptop, Postgres
or MySQL on a server. Other databases the compiler supports can follow by
adding their create statements.

**On a server** the admin supplies `store` and `secret_key`. If the key is
missing, the server stops with a message that includes a freshly generated key
to paste:

```text
DeepORG needs a secret key.
Add this to deeporg.yml, or set it as DEEPORG_SECRET_KEY, and start again:

  secret_key: 4f0c…e91a
```

Nobody has to work out how to make one.

**The store and your data.** The store can be the same database the agent
reads, if DeepORG may create its own schema there. If the data is read-only,
the store needs its own small database. This is the one requirement that cannot
be designed away, so setup says it up front.

**The setup link.** A new server on a public address must not let the first
visitor claim the admin account. On first start the server prints a one-time
setup link in its log, and serves nothing but setup and `/health` until an
admin exists. On a laptop the same link is opened automatically, so the
newcomer never sees it.

## Part 3: What has to be true underneath

Each item names the step it serves and the existing code it builds on. Claims
in this part come from reading the code unless they say otherwise.

### 3.1 One mode

*Serves: every step. This is why the generated config does not work today.*

Agentic becomes one behaviour everywhere.

- **No `production` flag in agentic.** A laptop and a server differ only in where the store is.
- **No saved-queries-only rule in agentic.** The agent writes queries; that is the product. Today `prodSec` is set from `conf.Production` in `core/api.go` (three places) and the lookup in `GraphQL` rejects any unnamed query, including the server's own. Only `Subscribe` exempts the trusted internal role (`subscriptionRequiresAllowList`, `core/subs.go`). Every store read goes through `internalStoreGraphQLOnce` (`serv/control_store_internal.go`), which is why one flag breaks watches, schedules, tasks and threads together.
- **No "dev defaults" in agentic.** The block in `serv/api.go` that logs "enabled by default (dev mode)" keys on `!Serv.Production`. It moves to keying on legacy dev mode only.
- **A database that fails to connect does not stop the server.** Today a failure in `initAllDBs` (`serv/init.go`) is fatal under `production`. In agentic the server starts without that source and reports it, so the admin can fix it on the page.
- **The `mcp.allow_*` switches stop being user-facing in agentic.** The rule becomes: the admin may change configuration, nobody else may.

### 3.2 The store opens first

*Serves: admin account, and keeping nothing on disk.*

DeepORG already has a store. It holds watches, tasks, threads, approvals,
notifications, usage and sign-in sessions, created by `ensureStoreTables`
(`serv/artifacts.go`). It also holds artifacts per owner: fragments, skills,
memory, annotations and agent profiles (`serv/artifact_overlay.go` and
neighbours). It runs on SQLite, Postgres or MySQL.

Today the store is opened from config (`initManagedArtifactStore`,
`serv/managed_artifacts.go`). That turns around: the store opens first, from
the start config alone, and everything else is read from it.

New tables, each following the pattern of `migrateAuthStoreSchema`
(`serv/auth_store.go`): a per-dialect create statement plus `ensureSQLColumn`
for later additions.

| Table | Holds | Replaces |
| :--- | :--- | :--- |
| `config` | The current configuration document and its earlier versions. | `dev.yml`, `prod.yml`, `agentic.yml` |
| `users` | Email, password hash, role, status. | Nothing; there is no user table today. |
| `tokens` | Personal access tokens, hashed. | Nothing. |
| `secrets` | Encrypted database passwords and model keys. | `secrets.enc.yml` |
| `discovery` | What DeepORG has learned about your tables. | The `.graphjin/discovery` directory |

The rest of the store keeps going through the compiler, as it does today.
These five tables are the exception, for two reasons. They are needed before
the compiler has a config to start from. And the ones that hold credentials
must not be reachable through GraphQL at all. So they are read with plain SQL,
the way `sqlAuthStore` reads sign-in sessions today, and are never listed in
`configStoreTableNames` (`serv/store_admin.go`). The plain SQL is small and is
written for the same three databases as the create statements.

**The store must be hidden from the data.** When the store shares a database
with a data source, its tables must be excluded from that source at discovery.
Today `isArtifactPhysicalTable` (`core/source_access.go`) only recognises six
tables, and only when the artifact source has the same name as the data source.
With every signed-in user able to read every table, anything less than
excluding the whole store schema would expose password hashes.

### 3.3 Nothing loose on disk

*Serves: running anywhere, and one place to back up.*

What is on disk today, and where it goes:

| Today | In this design |
| :--- | :--- |
| Config YAML files | The `config` table. |
| `secrets.enc.yml` | The `secrets` table. |
| `.graphjin/artifacts.sqlite3` | The store itself, wherever the start config points. |
| `.graphjin/discovery/` (schema snapshots, manifests, activation receipts) | The `discovery` table. |
| `queries/` and `workflows/` folders | Not read in agentic. They belong to the legacy flow. |
| Fragments, skills, memory | Already in the store as artifacts. No change. |

The discovery cache needs a note. It is refreshed in the background on an
interval (five minutes by default; `refreshLoop`, `serv/discovery_generations.go`),
and that stays. What changes is where a refreshed copy is kept: today each one
is a directory of files and the docs ask for a shared mount when there is more
than one server. Kept as rows in the store, every server sees the same copy and
the shared mount is no longer needed. A test install with two tables wrote
about 180 KB per copy; large schemas should be measured.

If semantic search is switched on, its vectors are files under the same
directory today. They move to the store with the rest. Semantic search is off
by default, so this can follow later.

### 3.4 Config in the store

*Serves: connecting a database and choosing a model without a file; changing them later.*

- The stored document uses today's config schema, so nothing about what can be configured changes. What changes is that users are not handed it.
- It is sparse: only what was set, not a dump of every default. Today's file writer (`saveConfigToDisk`, `serv/mcp_config.go`) writes the whole viper state back, which would turn defaults into explicit settings on the next load.
- It is loaded through `serv.NewConfig`, which already builds a full config with no file behind it.
- Changes go through the pipeline that exists: `handleUpdateCurrentConfig` validates, builds a staged runtime, connects and discovers a new database live, then commits. Preview-then-apply and the revision guard stay. The only new part is that a commit writes a new version of the document to the store.
- Anything that belongs in the start config is refused in the stored document.
- `deeporg config export` writes the stored document to a file for backup or review, and `deeporg config import` loads one. The store is the one source of truth.

### 3.5 One secret key

*Serves: sign-in, and storing passwords safely.*

Two keys are derived from the one secret key.

- **The session key** signs sign-in sessions. It is placed in `conf.Auth.JWT.Secret` at load, so `serv/console_session.go` and the token code need no change.
- **The sealing key** encrypts database passwords and model keys. The encryption in `serv/secret_keystore.go` (`localKeystore`) is reused; in agentic the encrypted values are kept in the `secrets` table instead of a YAML file.

Two gaps to close so setup can store what it collects:

- The keystore only decrypts values in the engine's config today. The model key (`ResolveAPIKey`, `agent/apikey.go`) can only come from an environment variable or a file. It needs to be readable from the store.
- Encrypted values carry a key id, so the secret key can be changed later without losing them.

### 3.6 Users and sign-in

*Serves: the admin account, teammates, connecting Claude Code.*

Today there is no user table. Identity comes only from token claims, and the
tokens DeepORG itself issues carry no role.

- **Passwords** are hashed with bcrypt, which `cmd` already uses (`hashPassword`, `cmd/cmd_seed.go`).
- **Password sign-in** is a new method beside the existing ones in `consoleSignInMethods` (`serv/console_session.go`). The demo sign-in (`mintDemoSession`) already shows the shape: the server issues a session for an identity it knows.
- **The session carries the user id.** Role and status are looked up from the `users` table on each request, with a short cache. Removing a user or changing their role takes effect at once, not when a token expires.
- **Invites** are one-time links the admin creates. The teammate chooses a password. There is no self-signup.
- **Personal access tokens** let the CLI and headless clients sign in. They are stored hashed.
- **Claude Code** signs in through the built-in OAuth flow against local users. That flow was written for an outside identity provider; it now also runs with a console session for a local account vouching for the person (slice 7 in Part 5). No personal access token is needed for it.
- **Sign-in attempts are rate limited.** The existing limiter (`serv/iplimiter.go`) tracks ten addresses with one global rate and is not suitable; this needs its own.

Single sign-on is not in the first version. When it is added it maps onto the
same `users` table.

### 3.7 The setup API

*Serves: the setup page and `deeporg setup`.*

- **Setup state is worked out, not stored.** Is there an admin? Is a database connected? Is a model key present? The page shows the first step that is not done.
- **One API, two clients.** The web page and the CLI call the same endpoints, so there is one implementation of each step.
- **Before an admin exists** the server answers only setup and `/health`, and the setup endpoints need the one-time token from the setup link.
- **After an admin exists** the same endpoints need an admin session. That is how a step is redone later.
- **`deeporg setup <url>`** runs the steps as prompts. It merges with today's `deeporg cli setup` (`cmd/cmd_setup.go`), which signs a CLI in to a server: if the server has no admin yet it runs setup, otherwise it signs in.
- **Sample data** is provided through a hook the CLI installs, the same way it installs the demo identity today (`demoSignInOption`, `cmd/demo_watches.go`). The demo logic stays where it is.
- **Suggested first questions** come from the catalog built at discovery. They are plain templates over table names, not a model call, so they appear even when the model step was skipped.

### 3.8 Default access

*Serves: the first question getting an answer.*

- Every signed-in user can read every table in a connected database.
- Writes and deletes stay off by default. Writes the agent proposes keep going through approvals.
- The admin limits access with table blocks, row filters and roles.

Where today's blocking comes from: `effectiveDatabaseAccess` (`core/config.go`)
gives every source `read: account`, uses a column named `account_id` to scope
rows, and blocks any table that has no such column. That is a rule for a
multi-tenant backend where every table carries a tenant id. A general database
agent cannot assume it. In agentic the default becomes `read: authenticated`
for every source, including ones in configs that exist today. Row scoping by a
column remains something the admin can set on a source; it is no longer assumed.

## Part 4: Backward compatibility

**The legacy flow does not change.** Which flow runs is decided in one place,
`setup` in `cmd/cmd.go`:

| What is found | What happens |
| :--- | :--- |
| `GO_ENV` is `dev` or `prod`, or a `config/dev.*` file exists | Legacy flow, unchanged. |
| A start config, or nothing at all | The new flow. If nothing is found, the start config is created. |
| `GO_ENV=agentic` with an `agentic.yml` | The server stops and says how to bring the file into the store with `deeporg config import`. |

`core/api.go` and `GetConfigName` are not changed.

**What changes for agentic installs that exist today:**

- They move from a YAML file to the store, with a one-time import.
- Tables that were blocked for want of an `account_id` column become readable by signed-in users.
- `queries/` and `workflows/` folders are no longer read.

The generated `agentic.yml` does not start today, so few installs can be
relying on it. The demos, the smoke suites and the evaluation harness do boot
agentic from YAML files (`examples/*/agentic.yml`). They switch to loading
their YAML into a fresh store at start, through the same import. That also
closes the gap that hid the current state: the demos will run the path
newcomers take.

## Part 5: Phases

Ordered by what a newcomer gets. Each phase ships on its own.

**Phase 1: A fresh agentic install works.**
Remove the conflicts in 3.1. Change the default access rule (3.8). Return an
error instead of crashing on a connection string that does not parse. Add a
test that boots from an empty directory. The path is still file-based after
this phase, but it works.

Done on 2026-11-24:

- Agentic ignores the `production` flag, in the engine (`productionEnabled`, `core/config.go`) and at config load (`normalizeConfigMode`, `serv/config.go`). A generated config now runs the same path the demos do.
- A signed-in user reads every table by default (`effectiveDatabaseAccess`). Writes stay off.
- The catalog needs a sign-in. It was readable with no token, which exposed every table and column name. The MCP catalog tool is now registered from config, not from what the anonymous startup caller may see (`registerCatalogTools`, `serv/mcp_catalog.go`).
- A Postgres connection string that cannot be read is an error that names the unfilled placeholder.
- The generated `agentic.yml` has no `production` line and carries a signing secret generated for that install.
- Only an admin reads `gj_config`. The table was mounted along with the catalog while its feature was off, so it had no access rules and any signed-in user could read it. A control-plane table whose root is off is no longer mounted (`registeredManagedTables`, `serv/control_plane_graphql.go`), and reading config is on in agentic for admins.
- `cmd/agentic_first_boot_test.go` boots the generated template and checks all of the above.

Left for later phases:

- **A person cannot sign in yet without making a token.** The generated config accepts tokens signed with its secret; accounts and a sign-in page arrive in Phase 2.
- **The "dev defaults" still apply to an agentic config that leaves `mcp.allow_*` unset.** The demos depend on four of them (raw queries, mutations, workflow execution, dev tools). They go away when those switches stop being user-facing, in Phase 2.

**Phase 2: Setup on a laptop.**
The start config and the store opening first. Admin account and password
sign-in. The setup page with the sample data, database and model steps. Config
and secrets kept in the store. Store tables hidden from data sources. Running
`deeporg` with no arguments starts the server. After this phase the
five-minute path is real.

Phase 2 is being built in slices.

*Slice 1, accounts and sign-in — done on 2026-11-25.*

- Accounts are kept in the store's existing sign-in table as rows of kind `account` (`serv/accounts.go`). That table already exists for SQLite, Postgres and MySQL and is never reachable through GraphQL, so no separate `users` table was needed. Section 3.2 lists one; this is the simpler form of the same thing.
- The first admin is created with `POST /api/v1/setup/admin`, which needs the one-time setup token. The server logs the setup link when it starts with no admin; `DEEPORG_SETUP_TOKEN` supplies the token where logs are not at hand.
- People sign in with `POST /api/v1/auth/session` and `{"method": "password"}`. Failed attempts are limited per address and per email.
- A session names the account and carries no role. The role and whether the account is active are read from the store on each request, cached for fifteen seconds (`withLocalAccounts`).
- `cmd/agentic_accounts_test.go` runs the whole path against the generated template.

Not in this slice: invites, personal access tokens, and holding back the rest of the API until an admin exists. The last one waits for the store-first boot, because the demos and tests sign in with hand-made tokens and have no admin account.

*Slice 2, the setup and sign-in screens — done on 2026-11-25.*

- While no admin exists the console shows one screen, whatever address it was opened at: "Create your admin account", with an email and a password (`serv/web/src/pages/Setup.jsx`). The setup link carries the token, so the newcomer is not asked for it; opened without the link, the screen asks for the token and says where to find it.
- After the account is created the console opens on the agent, signed in, with the token gone from the address.
- The sign-in sheet leads with email and password. The access-token form is still there behind "Use an access token instead".

*Slice 3, the start config and booting from the store — done on 2026-11-25.*

- Running `deeporg` with nothing after it starts the server.
- A new install is one with no `GO_ENV`, no `--path` and no `./config` directory. It takes the new flow (`usesStartConfig`, `cmd/start_config.go`). Everything else runs as before, including `GO_ENV=agentic` with an `agentic.yml`, which the demos, smokes and evaluation harness still use.
- The first run writes `~/.deeporg/deeporg.yml` with a generated secret key and a store beside it, and writes nothing to the directory it was started in. `DEEPORG_HOME` moves that folder; `DEEPORG_CONFIG` or a `deeporg.yml` in the current directory names a start config directly. `DEEPORG_STORE`, `DEEPORG_SECRET_KEY`, `DEEPORG_LISTEN` and `DEEPORG_LOG_LEVEL` override the file.
- A start config created on a laptop listens on `127.0.0.1:8080`, so a trial is not reachable from the network. A start config without `listen` uses `0.0.0.0:8080`.
- The server builds its configuration from a fixed base plus the document read from the store (`loadStartFlowConfig`). The document cannot set what belongs to the start config or decides who a caller is: `mode`, `production`, `host_port`, `auth`, `auth_fail_block`, `secrets`, `artifacts`.
- The key that signs sessions is derived from the secret key (`DeriveStoreSecret`), so sessions survive a restart and the sealing key, when it arrives, will be a different key.
- A server with no database connected starts and is usable: the admin can sign in and gets the whole console.
- `deeporg config import <file>` loads a configuration document into the store and `deeporg config export` prints it. The server reads the document when it starts.
- On a laptop the setup link is printed and opened in the browser once the server is listening.

What this slice does not do yet:

- **The store is a local SQLite file only.** A Postgres or MySQL store is refused with a message that says so. That is the server path, in Phase 3.
- **The configuration document sits in the same keyed table as the accounts**, not in a table of its own, and earlier versions are not kept.
- **Changes an admin makes while the server runs are not written to the store.** Config writes are still switched off in agentic. That, and encrypted secrets, come with the database and model steps.
- **The discovery cache is still a folder of files**, beside the store.
- **The rest of the API is still not held back until an admin exists.**

*Slice 4, the database step of setup — done on 2026-11-25.*

- After the admin step the console shows "Connect your data": a connection URL, or the details one by one (`serv/web/src/pages/SetupDatabase.jsx`). It can be left for later.
- `POST /api/v1/setup/database` needs an admin session. The server tests the connection before saving anything (`serv/setup_database.go`). A failure comes back as what went wrong and what to do, for a wrong host, a closed port, a refused password, a missing database, a database that needs encryption and a timeout.
- A connection that works is added to the stored configuration and the server restarts itself in place, reading it. The address, the store and the session all stay the same, and the console waits for it to come back.
- The stored configuration is encrypted at rest with a key derived from the secret key, because it now carries database passwords. Read with a different secret key, it is refused with a message that says so.
- Fixed along the way: the store's keyed table, which holds the accounts, was readable through GraphQL on a server with local accounts. It is now blocked for every role wherever it is in use.

What this slice does not do yet:

- **Counting the tables on the setup screen.** Done in slice 8.
- **Connecting a second database, or changing one, from the console.** Done in slice 8.
- **Changes take effect by restarting the process**, which drops requests in flight. That is fine during setup; changes made later should use the live path the config pipeline already has.
- **With no database connected, asking for a table that does not exist names the store's own tables in the error.** No data is exposed.

*Slice 5, the model step of setup — done on 2026-11-25.*

- After the database step the console shows "Choose a model": a provider (OpenAI, Anthropic or Google Gemini), its API key, and optionally a model name (`serv/web/src/pages/SetupModel.jsx`). It can be left for later, and the screen says what stays off without it.
- `POST /api/v1/setup/model` needs an admin session. The server checks the key by sending the provider one small request through the same client the agent uses (`CheckAPIKey`, `agent/check_key.go`). A refusal comes back as what went wrong: a key the provider does not accept, a model it does not have, an account with no usage left, a provider that cannot be reached. The key is never repeated back, even when the provider's own error contains it.
- A key that works is kept encrypted in the store, as its own record and not in the configuration document, so `deeporg config export` does not print it.
- On start, the key is handed to the agent in an environment variable of its own, `DEEPORG_MODEL_KEY`, which the stored configuration names. A provider variable already in the environment is neither used nor changed.

With this slice the path in Part 1 works from start to finish on a laptop: run `deeporg`, create the admin, connect a database, choose a model, ask a question.

What is still open in Phase 2:

- **`GJ_AGENT_*` variables in the environment override the stored model choice.** Found while testing. Fixed in slice 8.
- **The setup screens do not show which model a provider's default is.**
- **Hiding the store's table names** from the error for a table that does not exist, on a server with no database.

*Slice 6, sample data — done on 2026-11-25.*

- The "Connect your data" screen offers a second choice: "Use sample data". It connects the built-in demo company's database (accounts, users, subscriptions, invoices, payments, support tickets, usage events) the same way a database of the newcomer's own is connected.
- The data is built by the demo's own schema and seed code. That code works on the CLI's package globals, so the running server cannot call it in its own process: it runs the binary again with a hidden `sample-data` command and connects the file that command prints (`cmd/sample_data.go`). It takes two to three seconds.
- The demo keeps its own state in the same file; those tables are dropped, so only the company's data is left. The demo project is built in a scratch folder that is removed, and one file, `sample.db`, stays beside the store.
- The child process is given an environment with this install's start config, secrets and setting overrides taken out.

Known limits of the sample data:

- **Its dates do not move.** The demo, run with `--demo`, shifts its dates forward as days pass. The sample file is a plain database, so "last week" gets less true the longer it sits.
- **It is the database only.** The demo's policy files, its outside API and its saved routines are not connected.
- **There was no screen for connecting a database of your own afterwards.** Admin → Setup does it since slice 8.

*Slice 7, connecting Claude Code — done on 2026-11-25.*

An agent is given the server's address and nothing else. No secret is copied by hand.

- With local accounts and no other MCP sign-in configured, the MCP endpoint refuses a client that has not signed in and says where it can (`withAccountOAuthChallenge`, `serv/account_oauth.go`). Before this such a client was let through as nobody: it connected, every tool turned it away, and it never learned that signing in was the answer.
- The client registers itself, sends the person's browser to the server, and the console shows "Allow Claude Code to connect?" with the account it will act as and where it sends the browser afterwards (`serv/web/src/pages/ConnectApprove.jsx`). A client that is not on the person's own computer is named by its site, with a warning. The link survives sign-in, and setup on a server that has no admin yet.
- Allowing it gives the client a one-hour token and a 30-day one to renew it with. The token names the account and the client. The role is read from the account store on each request, as for a console session.
- **The token works at the MCP endpoint only.** GraphQL, REST and the console's own endpoints refuse it, so a connected agent can do what the MCP tools let it do and no more.
- The console has a Connect page with the command for Claude Code and the address for other agents (`serv/web/src/pages/Connect.jsx`). `deeporg mcp add claude <url>` works too and now says to finish in Claude Code with `/mcp`.
- This reuses the authorization-code and PKCE code written for single sign-on. Only who vouches for the person differs.
- It is off when the config sets `mcp.oauth`, defines an `anon` role, has no console, or is a demo with its own sign-in.

Two bugs found by running a real client:

- **Every tool disappeared for a strict client.** A field that can hold anything was described with the schema `true`. The MCP client library Claude Code is built on accepts only an object there and throws away the whole tool list. Such fields are now described as `{}`, which means the same.
- **`deeporg mcp add claude <url>` set up Codex.** The client named as an argument was read and then ignored. This is the command the server prints when it starts.

What it does not do:

- **A connected agent gets answers by asking the built-in agent**, which needs the model step done. Running queries itself (`mcp.allow_raw_queries`) is off on a new install, as it is for every agentic config. Without a model key the agent can search the catalog and nothing more. The Connect page says so and offers the model step.
- **It was checked with the MCP client library, not by clicking through Claude Code itself.**
- **Redirects must be `https`, or `http` to the person's own computer.** A client that hands the answer back through an address of its own kind, as some editors do, is refused.
- **There is no list of connected agents and no way to disconnect one** short of removing the account. The renewal token lasts 30 days, then the person is asked again.
- **A client that registers and is never allowed is forgotten after a week.** Registration and sign-in requests are rate limited per address.

*Slice 8, a simpler start — done on 2026-11-25.*

Each of these came from walking the finished flow as a newcomer.

- **The first run says one thing.** A new install prints the banner, which now reads DeepORG, and the setup link. The list of addresses, the two warnings a newcomer cannot act on and the second copy of the link are gone for an install that uses a start config; the start config's `log_level` brings the detail back. An install run from config files prints what it did before.
- **A taken port is not an error on a laptop.** If 8080 is in use the server moves to the next free port and says so. Where the address was chosen on purpose, through `DEEPORG_LISTEN` or for a server that listens beyond the machine, it stops and says what to change (`findListen`, `cmd/start_config.go`).
- **The setup screens say which step they are**, and the database step ends by saying how many tables it found before moving on.
- **The agent page suggests questions about the connected tables**, such as "How many accounts are there?" and "Which accounts have the most invoices?" (`serv/agent_starters.go`). Which table is the parent is read from the foreign keys.
- **With no model, the agent page offers the model step** to an admin, and tells anyone else to ask their admin. It no longer tells a newcomer to set an environment variable.
- **Setup can be come back to.** Admin → Setup lists the connected databases, the model and the people, and can connect another database, change the model and invite a teammate (`serv/web/src/pages/AdminSetup.jsx`, `serv/setup_manage.go`).
- **Teammates join by invite.** The admin enters an email and gets a link to send. The teammate chooses a password and is signed in as an ordinary user, or as an admin if the admin said so. A link works once, for a week; a new invite for the same email replaces the old one; only the link's hash is stored.
- **The model chosen on the setup page is the one used.** `GJ_AGENT_PROVIDER`, `GJ_AGENT_MODEL` and `GJ_AGENT_BASE_URL` left in the environment are dropped, with a warning, once a model has been chosen. Before that they still apply.
- **A setup step waits for the new server.** The reply to a step carries a marker that changes when the process restarts, so connecting a second database, whose flag was already clear, no longer returns before the restart.
- **The console and the terminal say DeepORG.**
- **The website quick start describes this flow**, and keeps the config-file path as a section for people who run DeepORG as an API service.

What this does not do:

- **There is no way to remove a person, change their role or reset a password** from the console.
- **Invite links are shown to the admin to send.** DeepORG sends no email.
- **Suggested questions are built from table names with fixed wording**, so an odd table name gives an odd question. A table one role may not read can still appear in a suggestion to that role.
- **A database cannot be disconnected from the console**, only added or replaced by name.

*Slice 9, two servers in place of three modes — done on 2026-11-25.*

The user's decisions: sources must be present, the API service has no MCP and no model, and it limits data with its own role rules as it always has.

- **There are two servers.** The agent server, and the API service, written `mode: api` (`prod` still means the same). `mode: dev` remains as the agent server with sign-in off, for demos and tests; it is not something a new user is pointed at.
- **The API service is an API and nothing else.** No agent, catalog, store or MCP server mounts there. The old setting that turned MCP back on in production is gone. With `production: false` a query is saved under its name and replaced when it changes; with `production: true` only saved queries are served (`TestAPIServiceServesSavedQueriesAndNothingAgentic`, `serv/api_service_test.go`).
- **The API service keeps its role rules with `sources:`.** Before, any config with sources refused `roles[].tables` and generated its rules from `sources[].access`. Now that is true of the agent server only (`IsAPIService`, `core/config.go`). An `access` block on a database source in the API service is a startup error.
- **A config file must have `sources:`.** One without is refused with the block to add, filled in from its old `database:` section and without its password (`missingSourcesError`, `serv/config.go`). The Go library still accepts its fields directly.
- **`deeporg serve new` makes an API service.** `dev.yml` and `prod.yml` set `mode: api` and declare the database under `sources:`. The agent, catalog and MCP settings are gone from them.

What this does not do:

- **The code still has the no-sources path**, for the Go library and for about 110 test files that build a config that way. Removing it means moving those tests to sources first.
- **A file named `dev.yml` with no `mode:` still means `mode: dev`.** Every demo relies on that.
- **A config with sources and no mode at all keeps its old locked-down posture**, with generated rules. It is not the API service.
- **The `mcp.allow_*` switches and the three default columns per capability are still there.**

*Slice 10, no separate store step — done on 2026-11-25.*

- `deeporg serve db store init` and `status` are removed, with the `artifacts.auto_init` setting they went with. DeepORG creates and migrates its own store tables each time it starts.
- A config that still says `auto_init: true` loads as before. One that says `auto_init: false` is refused with what to do: let the database user create tables in the store schema, and delete the setting.
- `deeporg serve db setup` no longer creates the store; starting the server does.

*Remaining slices:* hiding the store's tables from data sources.

**Phase 3: Nothing on disk, and setup on a server.**
The discovery cache in the store. The one-time setup link. `deeporg setup`
prompts. The container image starts the server. Demos and the evaluation
harness load through import. The new quick start is published.

**Phase 4: Teammates and Claude Code.**
Invites (done, slice 8). Personal access tokens. Claude Code sign-in against
local users (done, slice 7).

**Phase 5: Later.**
Telling other servers that config changed (a `config` entry in the existing
revision signals, `serv/revision_signal.go`). Changing the secret key.
Semantic search vectors in the store.

## Part 6: Open decisions

1. **The store on databases other than SQLite, Postgres and MySQL.** The compiler can talk to more. Each one needs create statements for the store's tables and the plain SQL for the five tables in 3.2. Recommendation: ship the first version on the three, and add others when someone needs one.
2. **Size of the discovery data in the store** on very large schemas. To be measured in Phase 3.

## Part 7: How each phase is checked

The checks follow `scripts/demo-smoke-all.sh --only default` (boot, restart,
confirm state survived) and `cmd/env_zero_file_test.go` (boot from an empty
directory).

| Phase | Check |
| :--- | :--- |
| 1 | Boot agentic from an empty directory against a real database. Sign in. `query_catalog` returns cards. A table with no `account_id` column is readable by a signed-in user. The watch and task loops log no errors for one minute. A connection string that does not parse returns an error. |
| 2 | Run `deeporg` in an empty directory. Finish setup through the API with sample data. Ask a question and get an answer. Restart; the admin, the database and the model key are still there. With the store sharing a database with the data, no role can read the `users` table. |
| 3 | Start with the start config given only as environment variables, a Postgres store and a read-only filesystem. Setup endpoints refuse a request without the one-time token. Finish setup with `deeporg setup`. Start a second server against the same store; it signs in and sees the same catalog without a shared mount. |
| 4 | Invite a teammate, accept, ask a question as them. Remove them; their next request is refused. Connect an MCP client with a personal access token. |

## Not in this design

- Platform-specific deploy files.
- Changes to the legacy `dev` and `prod` flow.
- Single sign-on in the first version.
- Email or chat delivery for invites. An invite is a link the admin sends themselves.
- A settings screen for how the agent works inside. Setup asks for a provider and a key and DeepORG chooses the rest.
