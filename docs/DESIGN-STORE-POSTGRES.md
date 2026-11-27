# Design: DeepORG's own data in Postgres

Status: step 1 is built (2026-11-27): one copy on Postgres, with no disk.
Steps 2 and 3 are proposed and not built. Written 2026-11-26.

What step 1 changed from this design: the secret key is required from the
environment when the store is Postgres and there is no start config file, and
is never made up, because a key kept only on a machine with no disk would be
lost with it. The store's schema is closed on every data source by
`isStoreSchemaTable` in `core/source_access.go`; without it the store's tables
were listed as data when its database was connected. Checked by
`TestStoreInPostgresNeedsNoDisk` in `cmd/postgres_store_test.go`, against a real
Postgres.

## The problem

A server set up in the browser keeps its own data in one local file,
`~/.deeporg/store.db`. That data is the accounts, invites, the connected
databases and their passwords, the model key, connected agents, chats, tasks,
watches and saved artifacts.

A local file has three costs:

1. **It needs a disk.** Hosting that gives a program no disk that survives a
   restart cannot run DeepORG this way.
2. **Only one copy can run.** Two servers cannot share the file, so there is no
   second copy to take over when the first one stops.
3. **Backups are a file copy** that the person running it has to arrange.

Keeping the same data in a Postgres database removes the first two and hands
the third to whatever already backs that database up.

## What a person does

On a server with no disk, two settings are enough:

```bash
DEEPORG_STORE=postgres://deeporg:password@db.internal:5432/deeporg
DEEPORG_SECRET_KEY=<a key of 32 characters or more>
```

Then `deeporg` starts, prints the setup link, and setup in the browser is the
same three steps as on a laptop. Nothing is written to the local disk.

If the key is missing, DeepORG already stops and prints a new one to paste
(`missingSecretKeyError` in `cmd/start_config.go`). That stays.

Starting a second copy with the same two settings gives a second server on the
same accounts, databases and model.

Nothing changes for an install that uses the file. The file stays the default,
because a laptop has no Postgres to point at.

## What exists today

Read from the code on 2026-11-26. Nothing here was run for this document.

- **The start config refuses a database server.** `startStorePath`
  (`cmd/start_config.go`) returns "the store must be a local file for now" for
  `postgres://` and `mysql://`.
- **The store is opened in two ways.**
  - Before the server starts, `openStoreSettings` (`serv/store_settings.go`)
    opens the SQLite file directly. It reads the stored configuration and the
    model key, and it is what `deeporg config import`, `config export` and
    `reset-password` use.
  - While the server runs, the same file is the "managed artifact store"
    (`serv/managed_artifacts.go`), registered as one of the server's databases.
- **The tables already have a Postgres form.** `authStoreDDL(dbType, schema)`
  and `artifactDDL(dbType, schema)` both branch on Postgres, and
  `sqlAuthStore.q` rewrites placeholders for it. A server run from config files
  can already keep this data in Postgres by naming a SQL source under
  `artifacts.source`. So the SQL is written; what is missing is the start flow
  using it.
- **A setup change restarts the process in place.** `restartInPlace` calls
  `syscall.Exec`, and the console waits for a new `setupBootID`. That reaches
  one process only.
- **Some state lives in one process.**
  - The sign-in limiter (`signInLimiter`). Its own comment says a second server
    multiplies the allowance.
  - The account cache, 15 seconds (`accountCacheTTL`). A removed account can
    keep working that long on another copy.
  - The setup token. Without `DEEPORG_SETUP_TOKEN`, each process makes its own
    random one (`initAccounts`).
- **Watch deliveries are claimed in the store** (`claimWatchDelivery`,
  `claimWatchEvent`), so two copies should not deliver one event twice. I have
  not run two copies to confirm it.
- **Two things still write local files:** the discovery cache
  (`discovery_cache.path`, `.graphjin/discovery` beside the store) and the
  sample company, which is a SQLite file in DeepORG's folder.

## The design

### 1. The start config accepts a Postgres address

`store:` in `deeporg.yml`, or `DEEPORG_STORE`, may be a `postgres://` or
`postgresql://` address. Anything else is a file, as now. MySQL stays refused:
one database server to support well is enough for a first version.

When both `DEEPORG_STORE` and `DEEPORG_SECRET_KEY` come from the environment,
DeepORG does not create or need `deeporg.yml`. Today a missing file is written
on first run, which fails on a disk that cannot be written.

### 2. One way to open the store

`openStoreSettings` takes the store setting instead of a path and returns the
keyed store for either kind. The server's managed store opens the same
setting. `Config.SetStore` carries the setting, not a path.

The tables go in the schema the config-file flow already uses (`_graphjin`),
so one set of DDL serves both ways of running DeepORG.

### 3. The store is never a database people query

The store's address is used for DeepORG's own tables and nothing else. It is
not offered as a database to connect, and its tables stay closed to GraphQL
for every role, as the keyed table is today (`keyedStoreInUse`).

If someone points `DEEPORG_STORE` at the same Postgres database they later
connect as data, the store's schema has to be left out of that source for
everyone. The first-run design already calls for this. It needs its own test:
no role, admin included, reads `_graphjin.auth_sessions` through any source.

### 4. More than one copy

- **Setup changes reach every copy.** Connecting a database, choosing a model
  or hiding a table writes the stored configuration and restarts the copy that
  took the request. Every copy checks the version of the stored configuration
  row every few seconds, one indexed read, and restarts itself in place when it
  has changed. Postgres `LISTEN` would be quicker but adds a connection that
  has to be kept alive; a five-second check is simpler and enough for a change
  an admin makes by hand.
- **One setup link for all copies.** While no admin exists and
  `DEEPORG_SETUP_TOKEN` is not set, the token is derived from the secret key
  (`DeriveStoreSecret(key, "setup")`) and is the same on every copy. It still
  stops working once an admin exists.
- **Left as it is for now, and said so in the docs:** the sign-in limit is per
  copy, and a removed account can keep working for up to 15 seconds on another
  copy.

### 5. With no disk

- The discovery cache goes to the system's temporary folder when the store is
  Postgres. It is rebuilt at start if it is not there.
- The sample company is not offered when the store is Postgres. Its file would
  be on one copy's disk and gone after a restart.

### 6. Moving from the file

One command copies everything from a file store into an empty Postgres store:

```bash
deeporg store copy ~/.deeporg/store.db postgres://deeporg:password@db.internal:5432/deeporg
```

It copies rows as they are, so it needs the same secret key on both sides,
because database passwords and the model key are encrypted with it. It refuses
a destination that already has an admin.

This is a new command, shortly after `store init` and `store status` were
removed. The difference is that those managed tables DeepORG now creates
itself, while this moves a person's data once. If one more command is not
wanted, the alternative is to offer no move at all: set the Postgres server up
again in the browser.

## What this does not do

- A MySQL store.
- Making the file store safe for two copies.
- A shared sign-in limit across copies. That needs a counter in the store or
  in Redis.
- Choosing the store in the browser. The store has to exist before the setup
  page can save anything, so it stays a start setting.

## How it is built and checked

Three steps, each one usable on its own.

1. **One copy on Postgres.** The start config accepts the address, the store
   opens either kind, and no file is needed when both settings come from the
   environment. Check: a `cmd` test that starts from an empty folder with a
   Postgres store (Docker, as the other database tests use), finishes setup
   through the API, asks for a table, restarts, and finds everything still
   there. `reset-password`, `config export` and `config import` run against it.
   A test that no role reads the store's tables when the data is in the same
   database.
2. **Two copies.** The configuration check and the shared setup link. Check:
   two services on one Postgres store in one test. An admin created on the
   first signs in on the second. A table hidden on the first is closed on the
   second within a few seconds. Both print the same setup link. One watch event
   is delivered once.
3. **Moving and docs.** The copy command, the "no disk" part of the Set Up And
   Deploy page, and the release notes.

## Decisions for the owner

1. **A move command, or none.** `deeporg store copy` as above, or tell people
   to set the new server up again.
2. **The schema name.** Keep `_graphjin`, which the config-file flow and
   existing installs use, or use `deeporg` for new Postgres stores and carry two
   names in the code.
3. **How quickly a change must reach other copies.** A check every five
   seconds, or `LISTEN` for near-instant at the cost of a held connection.
