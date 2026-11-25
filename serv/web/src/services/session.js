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

// setupState reads which first-run step, if any, the server is waiting on.
export function setupState(bootstrap) {
  return { needsAdmin: Boolean(bootstrap?.setup?.needs_admin) };
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
