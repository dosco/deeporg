import React from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { errorText } from "@/services/graphql";
import { acceptInvite, readInvite } from "@/services/session";
import { MIN_PASSWORD_LENGTH } from "./Setup";

// Invite is the page an invite link opens. It says who the invite is for and
// asks for one thing, a password. onDone runs once the account exists and
// this browser is signed in to it.
//
// A password link opens the same page. The account already exists, so the
// words change and the password replaces the old one.
// passwordLink says which of the two the address is for, so a link that no
// longer works is still described as what it was.
const Invite = ({ onDone, search = window.location.search, passwordLink = false }) => {
  // Read the token once: the address may change under this screen.
  const [token] = React.useState(() => new URLSearchParams(search).get("token") || "");
  const [email, setEmail] = React.useState("");
  const [reset, setReset] = React.useState(passwordLink);
  const [password, setPassword] = React.useState("");
  const [pending, setPending] = React.useState(false);
  const [error, setError] = React.useState("");
  const [gone, setGone] = React.useState(token ? "" : "This link is not complete. Ask whoever sent it to send it again.");

  React.useEffect(() => {
    if (!token) {
      return undefined;
    }
    let current = true;
    readInvite(token)
      .then((invite) => {
        if (current) {
          setReset(Boolean(invite.reset));
          setEmail(invite.email);
        }
      })
      .catch((err) => current && setGone(errorText(err, "This link could not be read.")));
    return () => {
      current = false;
    };
  }, [token]);

  const ready = password.length >= MIN_PASSWORD_LENGTH;

  async function submit(event) {
    event.preventDefault();
    if (!ready || pending) {
      return;
    }
    setPending(true);
    setError("");
    try {
      await acceptInvite(token, password);
      onDone?.();
    } catch (err) {
      setError(errorText(err, reset ? "The password could not be changed." : "The account could not be created."));
      setPending(false);
    }
  }

  return (
    <main id="main-content" className="relative mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-6 py-12">
      <p className="hud-label">{reset ? "Password" : "Invite"}</p>
      <h1 className="mt-2 text-2xl font-bold tracking-tight">{reset ? "Choose a new password" : "Join DeepORG"}</h1>
      {gone ? (
        <p role="alert" className="mt-3 text-sm text-destructive">{gone}</p>
      ) : !email ? (
        <p className="mt-3 text-sm text-muted-foreground">Reading your link…</p>
      ) : (
        <>
          <p className="mt-3 text-sm text-muted-foreground">
            {reset ? (
              <>This is for <span className="font-medium text-foreground">{email}</span>. Your old password stops working, and you are signed out everywhere else.</>
            ) : (
              <>You were invited as <span className="font-medium text-foreground">{email}</span>. Choose a password and you are in.</>
            )}
          </p>
          <form className="mt-8 grid gap-5" onSubmit={submit}>
            {/* Lets a password manager save the email with the password. */}
            <input type="email" value={email} readOnly hidden autoComplete="username" />
            <label className="grid gap-1.5 text-sm">
              <span className="font-medium">{reset ? "New password" : "Password"}</span>
              <Input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="new-password" autoFocus required />
              <span className="text-xs text-muted-foreground">At least {MIN_PASSWORD_LENGTH} characters.</span>
            </label>
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            <div>
              <Button type="submit" disabled={!ready || pending}>
                {reset ? (pending ? "Saving…" : "Save and sign in") : pending ? "Creating your account…" : "Create account"}
              </Button>
            </div>
          </form>
        </>
      )}
    </main>
  );
};

export default Invite;
