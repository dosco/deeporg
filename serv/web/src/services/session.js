import { fetchJSON } from "./graphql";

// Console sessions are HttpOnly cookies the server sets; this module only
// asks for one or ends it. The page never sees the token.

export function signInWithToken(token) {
  return startSession({ token });
}

export function signInAsDemo() {
  return startSession({ method: "demo" });
}

export function signInWithPassword(email, password) {
  return startSession({ method: "password", email, password });
}

// createFirstAdmin makes the first account on a new server and signs it in.
// The token comes from the setup link the server printed when it started.
export function createFirstAdmin({ token, email, password }) {
  return fetchJSON("/api/v1/setup/admin", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token, email, password }),
  });
}

// connectDatabase gives the server a database to test and, if it works, save.
// The server restarts with it, so follow this with waitForDatabase.
export function connectDatabase(connection) {
  return fetchJSON("/api/v1/setup/database", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(connection),
  });
}

// waitForDatabase resolves once the server is back up with a database
// connected. It resolves with the setup state, which says how many tables
// were found.
export function waitForDatabase(options) {
  return waitForSetupStep("needs_database", options);
}

// chooseModel gives the server a model provider and its key to check and, if
// the key works, save. The server restarts with it, so follow this with
// waitForModel.
export function chooseModel({ provider, apiKey, model }) {
  return fetchJSON("/api/v1/setup/model", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, api_key: apiKey, model }),
  });
}

// waitForModel resolves once the server is back up with the agent's key.
export function waitForModel(options) {
  return waitForSetupStep("needs_model", options);
}

// waitForSetupStep resolves with the setup state once the server reports the
// step is done. A setup step restarts the server; while it does, requests
// fail, which is expected. The server's reply to the step carries a marker
// that changes when it restarts, so a step whose own flag was already clear,
// such as connecting a second database, still waits for the new server.
async function waitForSetupStep(field, { boot = "", restarting = true, timeoutMs = 45000, intervalMs = 700 } = {}) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
    try {
      const state = await fetchJSON("/api/v1/setup");
      const restarted = !restarting || !boot || !state.boot || state.boot !== boot;
      if (restarted && !(field && state[field])) {
        return state;
      }
    } catch {
      // The server is restarting.
    }
  }
  throw new Error("That was saved, but the server has not come back yet. Reload this page in a moment.");
}

// setupDetails tells an admin what is connected and who has an account.
export function setupDetails() {
  return fetchJSON("/api/v1/setup/details");
}

// inviteTeammate makes a one-time link the admin sends to a teammate.
export function inviteTeammate(email, role) {
  return fetchJSON("/api/v1/setup/invites", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, role }),
  });
}

export function withdrawInvite(email) {
  return fetchJSON(`/api/v1/setup/invites?email=${encodeURIComponent(email)}`, { method: "DELETE" });
}

// readInvite says who an invite link is for.
export function readInvite(token) {
  return fetchJSON(`/api/v1/setup/invite?token=${encodeURIComponent(token)}`);
}

// acceptInvite creates the teammate's account with the password they chose
// and signs them in.
export function acceptInvite(token, password) {
  return fetchJSON("/api/v1/setup/invite", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token, password }),
  });
}

// manageState reads whether the caller is an admin who can manage this
// server from the console, and whether its databases and model are set there.
export function manageState(bootstrap) {
  return {
    canManage: Boolean(bootstrap?.manage),
    storeBacked: Boolean(bootstrap?.manage?.store_backed),
  };
}

// setupState reads which first-run step, if any, the server is waiting on.
export function setupState(bootstrap) {
  return {
    needsAdmin: Boolean(bootstrap?.setup?.needs_admin),
    needsDatabase: Boolean(bootstrap?.setup?.needs_database),
    needsModel: Boolean(bootstrap?.setup?.needs_model),
    sampleData: Boolean(bootstrap?.setup?.sample_data),
  };
}

// The admin can leave the model step of setup for later. That choice lasts for
// this browser tab and is kept under this key.
export const SKIP_MODEL_KEY = "deeporg.setup.skipModel";

// returnToModelStep undoes that choice and reloads, which brings the model
// step back for an admin.
export function returnToModelStep(location = window.location) {
  try {
    sessionStorage.removeItem(SKIP_MODEL_KEY);
  } catch {
    // Without storage the step was never remembered as skipped.
  }
  location.assign("/user/agent");
}

// connectState reads how a signed-in person connects their own agent. The
// address is absent on a server that does not sign agents in with an account.
// answers says how a connected agent gets answers from the data: "agent" when
// it asks the built-in agent, "queries" when it runs queries itself, and
// empty when it cannot do either yet.
export function connectState(bootstrap) {
  return {
    mcpURL: bootstrap?.connect?.mcp_url || "",
    answers: bootstrap?.connect?.answers || "",
    isAdmin: bootstrap?.identity?.role === "admin",
  };
}

// readConnectRequest describes an agent that is waiting to be allowed: what
// it calls itself and where it will send the browser afterwards.
export function readConnectRequest(request) {
  return fetchJSON(`/api/v1/oauth/consent?request=${encodeURIComponent(request)}`);
}

// answerConnectRequest allows or refuses the agent. The reply carries the
// address to send the browser back to, which is how the agent hears the answer.
export function answerConnectRequest(request, allow) {
  return fetchJSON("/api/v1/oauth/consent", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ request, allow }),
  });
}

export function signOut() {
  return fetchJSON("/api/v1/auth/session", { method: "DELETE" });
}

// consoleLoginURL sends the browser through single sign-on and back to the
// page it started from.
export function consoleLoginURL(loginURL, location = window.location) {
  const next = `${location.pathname}${location.search}`;
  return `${loginURL}?next=${encodeURIComponent(next)}`;
}

// signInState reads what the bootstrap says about signing in: whether this
// server needs a session at all, whether the caller has one, and how to start
// one.
export function signInState(bootstrap) {
  const identity = bootstrap?.identity || {};
  return {
    required: Boolean(identity.sign_in || identity.session),
    signedIn: Boolean(identity.session),
    name: identity.display_name || "",
    role: identity.role || "",
    methods: identity.sign_in?.methods || [],
    loginURL: identity.sign_in?.login_url || "",
  };
}

function startSession(body) {
  return fetchJSON("/api/v1/auth/session", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}
