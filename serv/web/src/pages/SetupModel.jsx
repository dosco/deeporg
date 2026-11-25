import React from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { chooseModel, waitForModel } from "@/services/session";

const PROVIDERS = [
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
  { value: "gemini", label: "Google Gemini" },
];

// SetupModel is the third screen of a new server: it gives the built-in agent
// a model to think with. The server checks the key with one small request
// before saving it, then restarts with it; onDone runs once it is back.
// onSkip leaves the step for later.
const SetupModel = ({ onDone, onSkip }) => {
  const [provider, setProvider] = React.useState("openai");
  const [apiKey, setAPIKey] = React.useState("");
  const [model, setModel] = React.useState("");
  const [nameModel, setNameModel] = React.useState(false);
  const [phase, setPhase] = React.useState("form"); // form | checking | starting
  const [error, setError] = React.useState("");

  const ready = apiKey.trim() !== "";
  const busy = phase !== "form";

  async function submit(event) {
    event.preventDefault();
    if (!ready || busy) {
      return;
    }
    setError("");
    setPhase("checking");
    try {
      await chooseModel({ provider, apiKey: apiKey.trim(), model: nameModel ? model.trim() : "" });
    } catch (err) {
      setError(err?.message || "The provider did not accept that key.");
      setPhase("form");
      return;
    }
    setPhase("starting");
    try {
      await waitForModel();
      onDone?.();
    } catch (err) {
      setError(err?.message || "The server has not come back yet.");
      setPhase("form");
    }
  }

  return (
    <main id="main-content" className="relative mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-6 py-12">
      <p className="hud-label">Setup</p>
      <h1 className="mt-2 text-2xl font-bold tracking-tight">Choose a model</h1>
      <p className="mt-3 text-sm text-muted-foreground">
        The built-in agent uses a model from a provider you have an account with. Paste an API key and we will check that it works.
      </p>
      <form className="mt-8 grid gap-5" onSubmit={submit}>
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">Provider</span>
          <Select value={provider} onChange={(event) => setProvider(event.target.value)} disabled={busy}>
            {PROVIDERS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
          </Select>
        </label>
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">API key</span>
          <Input type="password" value={apiKey} onChange={(event) => setAPIKey(event.target.value)} autoComplete="off" spellCheck={false} autoFocus disabled={busy} className="font-mono" />
          <span className="text-xs text-muted-foreground">It is kept encrypted on this server and is not shown again.</span>
        </label>
        {nameModel ? (
          <label className="grid gap-1.5 text-sm">
            <span className="font-medium">Model name</span>
            <Input value={model} onChange={(event) => setModel(event.target.value)} autoComplete="off" spellCheck={false} disabled={busy} className="font-mono" />
            <span className="text-xs text-muted-foreground">Leave it empty to use the provider's default.</span>
          </label>
        ) : (
          <button type="button" className="justify-self-start text-sm text-muted-foreground underline underline-offset-4" disabled={busy} onClick={() => setNameModel(true)}>
            Name a specific model
          </button>
        )}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        {phase === "starting" && <p role="status" className="text-sm text-muted-foreground">The key works. Starting the agent…</p>}
        <div>
          <Button type="submit" disabled={!ready || busy}>
            {phase === "checking" ? "Checking…" : phase === "starting" ? "Starting…" : "Check and save"}
          </Button>
        </div>
      </form>
      {onSkip && (
        <div className="mt-10 grid gap-1.5">
          <button type="button" className="justify-self-start text-left text-sm text-muted-foreground underline underline-offset-4" disabled={busy} onClick={onSkip}>
            Do this later
          </button>
          <p className="text-xs text-muted-foreground">
            Without a key the built-in agent and routines stay off. Everything else in the console works.
          </p>
        </div>
      )}
    </main>
  );
};

export default SetupModel;
