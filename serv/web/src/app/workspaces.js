export const workspaceDefinitions = {
  user: {
    id: "user",
    label: "User",
    description: "Your work",
    defaultPath: "/user/agent",
    items: [
      { path: "/user/agent", label: "Agent", layout: "canvas" },
      // Shown only when the server offers notifications to this viewer.
      { path: "/user/inbox", label: "Inbox", layout: "collection", requiresCapability: "gj_notification", count: "notifications" },
      { path: "/user/tasks", label: "Tasks", layout: "collection" },
      { path: "/user/watches", label: "Watches", layout: "collection" },
      { path: "/user/artifacts", label: "Artifacts", layout: "collection" },
      { path: "/user/memory", label: "Memory", layout: "collection" },
      { path: "/user/skills", label: "Skills", layout: "collection" },
      // Shown only when the server signs agents in with an account.
      { path: "/user/connect", label: "Connect", layout: "collection", requiresConnect: true },
    ],
  },
  admin: {
    id: "admin",
    label: "Admin",
    description: "Server",
    defaultPath: "/admin/runtime",
    items: [
      { path: "/admin/runtime", label: "Runtime", layout: "collection" },
      // Shown only to an admin on a server that holds accounts.
      { path: "/admin/setup", label: "Setup", layout: "collection", requiresManage: true },
      // The two views an API service has: a place to try queries, and the
      // reference for the endpoints it serves.
      { path: "/admin/workbench", label: "Workbench", layout: "canvas", apiService: true },
      { path: "/admin/catalog", label: "Catalog", layout: "collection" },
      { path: "/admin/workflows", label: "Workflows", layout: "collection" },
      // Shown only when the server serves its security report. A new install
      // does not: security.read is off by default on an agent server.
      { path: "/admin/security", label: "Security", layout: "collection", requiresCapability: "gj_security" },
      { path: "/admin/code", label: "Code", layout: "document" },
      { path: "/admin/config", label: "Config", layout: "document" },
      { path: "/admin/api", label: "API", layout: "document", apiService: true },
    ],
  },
  trainer: {
    id: "trainer",
    label: "Trainer",
    description: "Evals",
    defaultPath: "/trainer/reports",
    items: [
      { path: "/trainer/reports", label: "Reports", layout: "document" },
    ],
  },
};

export const defaultWorkspaceAvailability = ["user", "admin"];

// isAPIService reports whether the server is an API service and not an agent
// server.
export function isAPIService(bootstrap) {
  return bootstrap?.service === "api";
}

// homePath is where the console opens: the agent, or on an API service the
// Workbench.
export function homePath(bootstrap) {
  return isAPIService(bootstrap) ? "/admin/workbench" : "/user/agent";
}

export function workspaceIDFromPath(pathname) {
  const candidate = String(pathname || "").split("/").filter(Boolean)[0];
  return workspaceDefinitions[candidate] ? candidate : "user";
}

export function routeDefinition(pathname) {
  for (const workspace of Object.values(workspaceDefinitions)) {
    const item = workspace.items.find((candidate) => pathname.startsWith(candidate.path));
    if (item) {
      return { ...item, workspace };
    }
  }
  return { layout: "collection", workspace: workspaceDefinitions.user };
}

// workspaceServes reports whether the server granted a capability, such as a
// gj_ root, to a workspace. A page asks before it queries a root the server
// may not serve, so it can leave the question out.
export function workspaceServes(bootstrap, workspaceID, capability) {
  const served = (bootstrap?.workspaces || []).find((candidate) => candidate?.id === workspaceID);
  return (served?.capabilities || []).includes(capability);
}

// visibleItems drops items whose capability the server did not grant this
// workspace. With no bootstrap yet, gated items stay hidden.
export function visibleItems(workspace, bootstrap) {
  const served = (bootstrap?.workspaces || []).find((candidate) => candidate?.id === workspace?.id);
  const capabilities = new Set(served?.capabilities || []);
  // An API service has no agent, catalog or control plane, so only the views
  // that work without them are shown.
  const apiService = isAPIService(bootstrap);
  return (workspace?.items || []).filter((item) =>
    (!apiService || item.apiService) &&
    (!item.requiresCapability || capabilities.has(item.requiresCapability)) &&
    (!item.requiresConnect || Boolean(bootstrap?.connect?.mcp_url)) &&
    (!item.requiresManage || Boolean(bootstrap?.manage))
  );
}

export function availableWorkspaces(bootstrap) {
  const ids = Array.isArray(bootstrap?.workspaces)
    ? bootstrap.workspaces.map((workspace) => workspace?.id).filter(Boolean)
    : defaultWorkspaceAvailability;
  return ids.map((id) => workspaceDefinitions[id]).filter(Boolean);
}

export function commandItems(bootstrap) {
  return availableWorkspaces(bootstrap).flatMap((workspace) =>
    visibleItems(workspace, bootstrap).map((item) => ({ ...item, workspace }))
  );
}
