import React from "react";

import { Button } from "@/components/ui/button";
import SignInPanel from "@/components/Layout/SignInPanel";
import { errorText } from "@/services/graphql";
import { answerConnectRequest, readConnectRequest } from "@/services/session";

// ConnectApprove is where an agent such as Claude Code sends the browser when
// it wants to connect. The person signs in if they have not, is shown who is
// asking and where it will send them, and allows it or not. The agent hears
// the answer when the browser goes back to the address it gave.
const ConnectApprove = ({ request, session, onSignedIn, leave = (url) => window.location.assign(url) }) => {
  const [details, setDetails] = React.useState(null);
  const [error, setError] = React.useState("");
  const [pending, setPending] = React.useState("");
  const [answered, setAnswered] = React.useState("");

  React.useEffect(() => {
    if (!session.signedIn || !request) {
      return undefined;
    }
    let current = true;
    readConnectRequest(request)
      .then((found) => current && setDetails(found))
      .catch((err) => current && setError(errorText(err, "This request could not be read.")));
    return () => {
      current = false;
    };
  }, [request, session.signedIn]);

  async function answer(allow) {
    setPending(allow ? "allow" : "refuse");
    setError("");
    try {
      const reply = await answerConnectRequest(request, allow);
      setAnswered(allow ? "allowed" : "refused");
      leave(reply.redirect);
    } catch (err) {
      setError(errorText(err, "Your answer could not be saved."));
      setPending("");
    }
  }

  const frame = (children) => (
    <main id="main-content" className="relative mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-6 py-12">
      <p className="hud-label">Connect</p>
      {children}
    </main>
  );

  if (!request) {
    return frame(
      <>
        <h1 className="mt-2 text-2xl font-bold tracking-tight">Nothing is waiting to connect</h1>
        <p className="mt-3 text-sm text-muted-foreground">Start again from the app you are connecting. It opens this page with its request.</p>
      </>
    );
  }

  if (!session.signedIn) {
    return frame(
      <>
        <h1 className="mt-2 text-2xl font-bold tracking-tight">Sign in to connect your app</h1>
        <p className="mt-3 text-sm text-muted-foreground">An app is asking to connect to DeepORG. Sign in and you will be shown what it is.</p>
        <SignInPanel state={session} onChange={onSignedIn} intro="" />
      </>
    );
  }

  if (answered) {
    return frame(
      <>
        <h1 className="mt-2 text-2xl font-bold tracking-tight">{answered === "allowed" ? "Connected" : "Not connected"}</h1>
        <p role="status" className="mt-3 text-sm text-muted-foreground">You can go back to the app now.</p>
      </>
    );
  }

  if (!details) {
    return frame(
      <>
        <h1 className="mt-2 text-2xl font-bold tracking-tight">Connect an app</h1>
        {error
          ? <p role="alert" className="mt-3 text-sm text-destructive">{error}</p>
          : <p className="mt-3 text-sm text-muted-foreground">Reading the request…</p>}
      </>
    );
  }

  return frame(
    <>
      <h1 className="mt-2 text-2xl font-bold tracking-tight">Allow {details.client_name} to connect?</h1>
      <p className="mt-3 text-sm text-muted-foreground">
        It will act as <span className="font-medium text-foreground">{details.email}</span>. It can see and do what you can in DeepORG.
      </p>
      {details.on_this_computer ? (
        <p className="mt-3 text-sm text-muted-foreground">It is an app on this computer.</p>
      ) : (
        <p className="mt-3 text-sm text-muted-foreground">
          After you answer, you are sent to <span className="font-mono text-foreground">{details.destination}</span>. Allow this only if you started it yourself and trust that site.
        </p>
      )}
      {error && <p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}
      <div className="mt-8 flex items-center gap-5">
        <Button type="button" disabled={pending !== ""} onClick={() => answer(true)}>
          {pending === "allow" ? "Connecting…" : "Allow"}
        </Button>
        <button type="button" className="text-sm text-muted-foreground underline underline-offset-4" disabled={pending !== ""} onClick={() => answer(false)}>
          Don't allow
        </button>
      </div>
    </>
  );
};

export default ConnectApprove;
