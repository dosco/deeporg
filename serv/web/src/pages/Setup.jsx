import React from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { errorText } from "@/services/graphql";
import { createFirstAdmin } from "@/services/session";

export const MIN_PASSWORD_LENGTH = 10;

// Setup is the first screen of a new server: it creates the admin account.
// The setup link carries a one-time token, which is what stops the first
// stranger to reach a new server from claiming it. onDone runs once the
// account exists and the browser holds its session.
const Setup = ({ onDone, search = window.location.search }) => {
  // Read the link's token once. The address may change under this screen, and
  // a newcomer who arrived with a token must never be asked to type it.
  const [linkToken] = React.useState(() => new URLSearchParams(search).get("token") || "");
  const [token, setToken] = React.useState(linkToken);
  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [pending, setPending] = React.useState(false);
  const [error, setError] = React.useState("");

  const ready = token.trim() !== "" && email.trim() !== "" && password.length >= MIN_PASSWORD_LENGTH;

  async function submit(event) {
    event.preventDefault();
    if (!ready || pending) {
      return;
    }
    setPending(true);
    setError("");
    try {
      await createFirstAdmin({ token: token.trim(), email: email.trim(), password });
      onDone?.();
    } catch (err) {
      setError(errorText(err, "The account could not be created."));
      setPending(false);
    }
  }

  return (
    <main id="main-content" className="relative mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-6 py-12">
      <p className="hud-label">Setup · Step 1 of 3</p>
      <h1 className="mt-2 text-2xl font-bold tracking-tight">Create your admin account</h1>
      <p className="mt-3 text-sm text-muted-foreground">
        This is the first account on this server. You use it to sign in and to change settings later.
      </p>
      <form className="mt-8 grid gap-5" onSubmit={submit}>
        {!linkToken && (
          <label className="grid gap-1.5 text-sm">
            <span className="font-medium">Setup token</span>
            <Input value={token} onChange={(event) => setToken(event.target.value)} autoComplete="off" spellCheck={false} className="font-mono" />
            <span className="text-xs text-muted-foreground">It is in the setup link this server printed when it started.</span>
          </label>
        )}
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">Email</span>
          <Input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" autoFocus required />
        </label>
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">Password</span>
          <Input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="new-password" minLength={MIN_PASSWORD_LENGTH} required />
          <span className="text-xs text-muted-foreground">At least {MIN_PASSWORD_LENGTH} characters.</span>
        </label>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <div>
          <Button type="submit" disabled={!ready || pending}>
            {pending ? "Creating…" : "Create account"}
          </Button>
        </div>
      </form>
    </main>
  );
};

export default Setup;
