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
// connected.
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

// waitForSetupStep resolves once the server reports the step is done. A setup
// step restarts the server; while it does, requests fail, which is expected.
async function waitForSetupStep(field, { timeoutMs = 45000, intervalMs = 700 } = {}) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
    try {
      const state = await fetchJSON("/api/v1/setup");
      if (!state[field]) {
        return;
      }
    } catch {
      // The server is restarting.
    }
  }
  throw new Error("That was saved, but the server has not come back yet. Reload this page in a moment.");
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
