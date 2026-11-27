import React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { DataErrorState, LoadingState } from "@/components/ui";
import { useConsole } from "@/components/Layout/console-context";
import { errorText } from "@/services/graphql";
import { changeRole, inviteTeammate, makePasswordLink, manageState, removePerson, setupDetails, withdrawInvite } from "@/services/session";
import SetupDatabase from "./SetupDatabase";
import SetupModel from "./SetupModel";

// AdminSetup is where an admin comes back to what first-run setup did: the
// databases that are connected, the model the agent uses, and who has an
// account. Each can be added to or changed here.
const AdminSetup = () => {
  const { bootstrap } = useConsole();
  const queryClient = useQueryClient();
  const { canManage, storeBacked } = manageState(bootstrap);
  const [editing, setEditing] = React.useState(""); // "" | database | model
  const details = useQuery({ queryKey: ["setup-details"], queryFn: setupDetails, enabled: canManage, staleTime: 0 });

  if (!canManage) {
    return (
      <div className="max-w-2xl">
        <h1 className="text-2xl font-bold tracking-tight">Setup</h1>
        <p className="mt-3 text-sm text-muted-foreground">Only an admin signed in with a DeepORG account can change this server's setup.</p>
      </div>
    );
  }

  // The server restarted with the change, so everything the console knew
  // about it is out of date.
  function changed() {
    setEditing("");
    queryClient.clear();
    window.location.reload();
  }

  if (editing === "database") {
    return <SetupDatabase embedded askName onDone={changed} onSkip={() => setEditing("")} />;
  }
  if (editing === "model") {
    return <SetupModel embedded onDone={changed} onSkip={() => setEditing("")} />;
  }
  if (details.isLoading) {
    return <LoadingState />;
  }
  if (details.error) {
    return <DataErrorState error={details.error} />;
  }

  const { databases = [], model = {}, people = [], invites = [] } = details.data || {};
  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-bold tracking-tight">Setup</h1>
      <p className="mt-3 text-sm text-muted-foreground">What this server is connected to, and who can sign in to it.</p>

      {storeBacked && (
        <section className="mt-10" aria-labelledby="setup-data">
          <h2 id="setup-data" className="text-base font-bold">Your data</h2>
          {databases.length === 0 ? (
            <p className="mt-3 text-sm text-muted-foreground">No database is connected yet.</p>
          ) : (
            <ul className="mt-3 grid gap-1.5 text-sm">
              {databases.map((database) => (
                <li key={database.name} className="flex flex-wrap items-baseline gap-x-3">
                  <span className="font-mono">{database.name}</span>
                  <span className="text-muted-foreground">{database.type}, {database.tables} {database.tables === 1 ? "table" : "tables"}</span>
                </li>
              ))}
            </ul>
          )}
          <div className="mt-4">
            <Button type="button" variant="secondary" onClick={() => setEditing("database")}>
              {databases.length === 0 ? "Connect a database" : "Connect another database"}
            </Button>
          </div>
        </section>
      )}

      {storeBacked && (
        <section className="mt-10" aria-labelledby="setup-model">
          <h2 id="setup-model" className="text-base font-bold">Model</h2>
          <p className="mt-3 text-sm text-muted-foreground">
            {model.ready
              ? <>The built-in agent uses <span className="font-mono text-foreground">{model.provider}</span>{model.model ? <>, model <span className="font-mono text-foreground">{model.model}</span></> : ", with the provider's default model"}.</>
              : "The built-in agent has no model yet, so it is off."}
          </p>
          <div className="mt-4">
            <Button type="button" variant="secondary" onClick={() => setEditing("model")}>
              {model.ready ? "Change the model" : "Choose a model"}
            </Button>
          </div>
        </section>
      )}

      <People people={people} invites={invites} onChange={() => details.refetch()} />
    </div>
  );
};

// People lists who has an account and lets the admin invite someone, change
// what a person may do, give them a link for a new password, or remove them.
// An invite and a password link are both links the admin sends; the person
// chooses their own password.
function People({ people, invites, onChange }) {
  const [email, setEmail] = React.useState("");
  const [role, setRole] = React.useState("user");
  const [pending, setPending] = React.useState(false);
  const [error, setError] = React.useState("");
  // made is the link to send: { kind: "invite" | "password", email, link }.
  const [made, setMade] = React.useState(null);
  const [copied, setCopied] = React.useState(false);
  // The person the admin is about to remove, until they confirm or back out.
  const [removing, setRemoving] = React.useState("");

  async function invite(event) {
    event.preventDefault();
    if (!email.trim() || pending) {
      return;
    }
    setPending(true);
    setError("");
    setCopied(false);
    try {
      setMade({ kind: "invite", ...(await inviteTeammate(email.trim(), role)) });
      setEmail("");
      onChange?.();
    } catch (err) {
      setError(errorText(err, "The invite could not be made."));
    } finally {
      setPending(false);
    }
  }

  async function withdraw(address) {
    setError("");
    try {
      await withdrawInvite(address);
      if (made?.email === address) {
        setMade(null);
      }
      onChange?.();
    } catch (err) {
      setError(errorText(err, "The invite could not be withdrawn."));
    }
  }

  async function setPersonRole(address, next) {
    setError("");
    try {
      await changeRole(address, next);
      onChange?.();
    } catch (err) {
      setError(errorText(err, "The role could not be changed."));
    }
  }

  async function passwordLink(address) {
    setError("");
    setCopied(false);
    try {
      setMade({ kind: "password", ...(await makePasswordLink(address)) });
    } catch (err) {
      setError(errorText(err, "The link could not be made."));
    }
  }

  async function remove(address) {
    setError("");
    try {
      await removePerson(address);
      setRemoving("");
      if (made?.email === address) {
        setMade(null);
      }
      onChange?.();
    } catch (err) {
      setError(errorText(err, "The person could not be removed."));
    }
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(made.link);
      setCopied(true);
    } catch {
      // Without clipboard access the link is still there to select.
    }
  }

  const quiet = "text-muted-foreground underline underline-offset-4 hover:text-foreground";
  return (
    <section className="mt-10" aria-labelledby="setup-people">
      <h2 id="setup-people" className="text-base font-bold">People</h2>
      <ul className="mt-3 grid gap-2.5 text-sm">
        {people.map((person) => (
          <li key={person.email} className="flex flex-wrap items-center gap-x-4 gap-y-1.5">
            <span>{person.email}</span>
            {person.you ? (
              // An admin cannot demote or remove their own account, so that a
              // server is never left with nobody in charge.
              <span className="text-muted-foreground">{person.role === "admin" ? "admin" : "can use DeepORG"}, you</span>
            ) : removing === person.email ? (
              <span className="flex flex-wrap items-center gap-x-4 gap-y-1.5">
                <span>Remove them? They are signed out at once.</span>
                <button type="button" className="font-medium text-destructive underline underline-offset-4" onClick={() => remove(person.email)}>
                  Remove {person.email}
                </button>
                <button type="button" className={quiet} onClick={() => setRemoving("")}>
                  Keep
                </button>
              </span>
            ) : (
              <>
                <Select aria-label={`What ${person.email} can do`} className="w-auto" value={person.role === "admin" ? "admin" : "user"} onChange={(event) => setPersonRole(person.email, event.target.value)}>
                  <option value="user">can use DeepORG</option>
                  <option value="admin">is an admin</option>
                </Select>
                <button type="button" className={quiet} aria-label={`New password link for ${person.email}`} onClick={() => passwordLink(person.email)}>
                  New password link
                </button>
                <button type="button" className={quiet} aria-label={`Remove ${person.email}`} onClick={() => setRemoving(person.email)}>
                  Remove
                </button>
              </>
            )}
          </li>
        ))}
        {invites.map((invite) => (
          <li key={`invite:${invite.email}`} className="flex flex-wrap items-baseline gap-x-4">
            <span>{invite.email}</span>
            <span className="text-muted-foreground">invited, has not joined yet</span>
            <button type="button" className={quiet} onClick={() => withdraw(invite.email)}>
              Withdraw
            </button>
          </li>
        ))}
      </ul>

      <form className="mt-6 grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-end" onSubmit={invite}>
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">Invite a teammate</span>
          <Input type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="name@company.com" autoComplete="off" />
        </label>
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">They</span>
          <Select value={role} onChange={(event) => setRole(event.target.value)}>
            <option value="user">can use DeepORG</option>
            <option value="admin">are an admin too</option>
          </Select>
        </label>
        <Button type="submit" variant="secondary" disabled={!email.trim() || pending}>
          {pending ? "Making the link…" : "Make an invite link"}
        </Button>
      </form>
      {error && <p role="alert" className="mt-3 text-sm text-destructive">{error}</p>}
      {made && (
        <div role="status" className="mt-5 grid gap-2 text-sm">
          {made.kind === "password" ? (
            <p>Send this link to {made.email}. It works once, for 24 hours. They choose a new password and are signed out everywhere else.</p>
          ) : (
            <p>Send this link to {made.email}. It works once, for a week. They choose their own password.</p>
          )}
          <div className="flex items-start gap-4">
            <code aria-label={made.kind === "password" ? "Password link" : "Invite link"} className="min-w-0 flex-1 select-all break-all font-mono text-[0.8125rem] leading-6">{made.link}</code>
            <button type="button" className={`shrink-0 ${quiet}`} onClick={copy}>
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
        </div>
      )}
    </section>
  );
}

export default AdminSetup;
