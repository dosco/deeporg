import React from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { connectDatabase, waitForDatabase } from "@/services/session";

const KINDS = [
  { value: "postgres", label: "PostgreSQL", port: "5432" },
  { value: "mysql", label: "MySQL", port: "3306" },
  { value: "mssql", label: "SQL Server", port: "1433" },
  { value: "sqlite", label: "SQLite", port: "" },
];

// SetupDatabase is the second screen of a new server: it connects the
// database the agent answers questions from. The server tests the connection
// before saving it, then restarts with it; onDone runs once it is back.
// onSkip leaves the step for later. With sampleData, the screen also offers
// a sample company's data to someone with no database to hand. step, such as
// "Step 2 of 3", is shown while this is part of first-run setup. After
// setup an admin connects another database with the same screen: askName
// adds a name for it, and embedded lays it out as part of a console page.
const SetupDatabase = ({ onDone, onSkip, sampleData = false, step = "", askName = false, embedded = false }) => {
  const [name, setName] = React.useState("");
  const [byParts, setByParts] = React.useState(false);
  const [url, setURL] = React.useState("");
  const [parts, setParts] = React.useState({ type: "postgres", host: "", port: "", database: "", user: "", password: "", path: "" });
  const [phase, setPhase] = React.useState("form"); // form | testing | sample | starting | connected
  const [error, setError] = React.useState(null);
  const [tables, setTables] = React.useState(null);

  const setPart = (key) => (event) => setParts((current) => ({ ...current, [key]: event.target.value }));
  const isFile = parts.type === "sqlite";
  const ready = (!askName || name.trim() !== "") && (byParts
    ? (isFile ? parts.path.trim() !== "" : parts.host.trim() !== "" && parts.database.trim() !== "")
    : url.trim() !== "");

  async function submit(event) {
    event.preventDefault();
    if (!ready || phase !== "form") {
      return;
    }
    setError(null);
    setPhase("testing");
    const connection = byParts
      ? (isFile
        ? { type: parts.type, path: parts.path.trim() }
        : { type: parts.type, host: parts.host.trim(), port: Number(parts.port) || 0, database: parts.database.trim(), user: parts.user.trim(), password: parts.password })
      : { url: url.trim() };
    if (askName) {
      connection.name = name.trim().toLowerCase();
    }
    let reply;
    try {
      reply = await connectDatabase(connection);
    } catch (err) {
      setError({ message: err?.message || "We could not connect to that database." });
      setPhase("form");
      return;
    }
    await started(reply);
  }

  // started waits for the server to come back connected, then says what it
  // found before moving on, so the person knows the connection really works.
  async function started(reply) {
    setPhase("starting");
    try {
      const state = await waitForDatabase({ boot: reply?.boot, restarting: reply?.restarting !== false });
      setTables(typeof state?.tables === "number" ? state.tables : null);
      setPhase("connected");
    } catch (err) {
      setError({ message: err?.message || "The server has not come back yet." });
      setPhase("form");
    }
  }

  async function useSample() {
    if (phase !== "form") {
      return;
    }
    setError(null);
    setPhase("sample");
    let reply;
    try {
      reply = await connectDatabase({ sample: true });
    } catch (err) {
      setError({ message: err?.message || "The sample data could not be prepared." });
      setPhase("form");
      return;
    }
    await started(reply);
  }

  const label = step ? `Setup · ${step}` : "Setup";
  // On its own the screen fills the page. Inside the console it is a section.
  const Frame = embedded ? "div" : "main";
  const frame = embedded
    ? { className: "w-full max-w-md" }
    : { id: "main-content", className: "relative mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-6 py-12" };
  if (phase === "connected") {
    return (
      <Frame {...frame}>
        <p className="hud-label">{label}</p>
        <h1 className="mt-2 text-2xl font-bold tracking-tight">Your data is connected</h1>
        <p role="status" className="mt-3 text-sm text-muted-foreground">
          {tables === null
            ? "The connection works."
            : tables === 0
              ? "The connection works, but we found no tables. Check that this database user is allowed to see them."
              : `We found ${tables} ${tables === 1 ? "table" : "tables"}.`}
        </p>
        <div className="mt-8">
          <Button type="button" autoFocus onClick={() => onDone?.()}>Continue</Button>
        </div>
      </Frame>
    );
  }

  const busy = phase !== "form";
  return (
    <Frame {...frame}>
      <p className="hud-label">{label}</p>
      <h1 className="mt-2 text-2xl font-bold tracking-tight">{askName ? "Connect another database" : "Connect your data"}</h1>
      <p className="mt-3 text-sm text-muted-foreground">
        The agent answers questions from your database. Give it a connection and we will check that it works.
      </p>
      <form className="mt-8 grid gap-5" onSubmit={submit}>
        {askName && (
          <label className="grid gap-1.5 text-sm">
            <span className="font-medium">Name</span>
            <Input value={name} onChange={(event) => setName(event.target.value)} autoComplete="off" spellCheck={false} placeholder="billing" disabled={busy} className="font-mono" />
            <span className="text-xs text-muted-foreground">A short name for this database, in lowercase letters. Using the name of one that is already connected replaces it.</span>
          </label>
        )}
        {!byParts && (
          <label className="grid gap-1.5 text-sm">
            <span className="font-medium">Connection URL</span>
            <Input value={url} onChange={(event) => setURL(event.target.value)} placeholder="postgres://user:password@host:5432/database" autoComplete="off" spellCheck={false} autoFocus disabled={busy} className="font-mono" />
          </label>
        )}
        {byParts && (
          <>
            <label className="grid gap-1.5 text-sm">
              <span className="font-medium">Kind of database</span>
              <Select value={parts.type} onChange={setPart("type")} disabled={busy}>
                {KINDS.map((kind) => <option key={kind.value} value={kind.value}>{kind.label}</option>)}
              </Select>
            </label>
            {isFile ? (
              <label className="grid gap-1.5 text-sm">
                <span className="font-medium">File path</span>
                <Input value={parts.path} onChange={setPart("path")} autoComplete="off" spellCheck={false} disabled={busy} className="font-mono" />
              </label>
            ) : (
              <>
                <div className="grid grid-cols-[1fr_6rem] gap-3">
                  <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">Host</span>
                    <Input value={parts.host} onChange={setPart("host")} autoComplete="off" spellCheck={false} disabled={busy} />
                  </label>
                  <label className="grid gap-1.5 text-sm">
                    <span className="font-medium">Port</span>
                    <Input value={parts.port} onChange={setPart("port")} inputMode="numeric" placeholder={KINDS.find((kind) => kind.value === parts.type)?.port} autoComplete="off" disabled={busy} />
                  </label>
                </div>
                <label className="grid gap-1.5 text-sm">
                  <span className="font-medium">Database name</span>
                  <Input value={parts.database} onChange={setPart("database")} autoComplete="off" spellCheck={false} disabled={busy} />
                </label>
                <label className="grid gap-1.5 text-sm">
                  <span className="font-medium">User</span>
                  <Input value={parts.user} onChange={setPart("user")} autoComplete="off" spellCheck={false} disabled={busy} />
                </label>
                <label className="grid gap-1.5 text-sm">
                  <span className="font-medium">Database password</span>
                  <Input type="password" value={parts.password} onChange={setPart("password")} autoComplete="off" disabled={busy} />
                </label>
              </>
            )}
          </>
        )}
        {error && <p role="alert" className="text-sm text-destructive">{error.message}</p>}
        {phase === "sample" && <p role="status" className="text-sm text-muted-foreground">Loading the sample data…</p>}
        {phase === "starting" && <p role="status" className="text-sm text-muted-foreground">Connected. Starting up with your database…</p>}
        <div className="flex items-center gap-5">
          <Button type="submit" disabled={!ready || busy}>
            {phase === "testing" ? "Checking…" : phase === "starting" ? "Starting…" : "Connect"}
          </Button>
          <button type="button" className="text-sm text-muted-foreground underline underline-offset-4" disabled={busy} onClick={() => { setByParts((current) => !current); setError(null); }}>
            {byParts ? "Paste a connection URL instead" : "Enter the details separately"}
          </button>
        </div>
      </form>
      {sampleData && (
        <div className="mt-10 grid gap-2">
          <p className="text-sm font-medium">No database to hand?</p>
          <p className="text-sm text-muted-foreground">
            Try it with a sample company: its accounts, subscriptions, invoices and support tickets.
          </p>
          <div>
            <Button type="button" variant="secondary" disabled={busy} onClick={useSample}>
              {phase === "sample" ? "Loading…" : "Use sample data"}
            </Button>
          </div>
        </div>
      )}
      {onSkip && (
        <button type="button" className="mt-10 justify-self-start text-left text-sm text-muted-foreground underline underline-offset-4" disabled={busy} onClick={onSkip}>
          {embedded ? "Cancel" : "Do this later"}
        </button>
      )}
    </Frame>
  );
};

export default SetupDatabase;
