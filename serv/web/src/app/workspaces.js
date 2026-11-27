import {
  Bot,
  BookOpenText,
  Braces,
  Brain,
  Code2,
  FileCheck2,
  FileCog,
  FlaskConical,
  Gauge,
  GraduationCap,
  Inbox,
  Library,
  ListTodo,
  Plug,
  Radar,
  ServerCog,
  Settings2,
  ShieldCheck,
  SquareTerminal,
  UserRound,
  Workflow,
} from "lucide-react";

// Every view carries an icon and one of the palette's five section tones
// (warm, amber, blue, teal, violet), taken from the deeporg.io palette. The rail, the search palette and the
// page header all read them from here, so a view looks the same everywhere.
export const workspaceDefinitions = {
  user: {
    id: "user",
    label: "User",
    description: "Your work",
    icon: UserRound,
    tone: "warm",
    defaultPath: "/user/agent",
    items: [
      { path: "/user/agent", label: "Agent", layout: "canvas", icon: Bot, tone: "warm" },
      // Shown only when the server offers notifications to this viewer.
      { path: "/user/inbox", label: "Inbox", layout: "collection", icon: Inbox, tone: "amber", requiresCapability: "gj_notification", count: "notifications" },
      { path: "/user/tasks", label: "Tasks", layout: "collection", icon: ListTodo, tone: "teal" },
      { path: "/user/watches", label: "Watches", layout: "collection", icon: Radar, tone: "blue" },
      { path: "/user/artifacts", label: "Artifacts", layout: "collection", icon: FileCheck2, tone: "violet" },
      { path: "/user/memory", label: "Memory", layout: "collection", icon: Brain, tone: "amber" },
      { path: "/user/skills", label: "Skills", layout: "collection", icon: BookOpenText, tone: "blue" },
      // Shown only when the server signs agents in with an account.
      { path: "/user/connect", label: "Connect", layout: "collection", icon: Plug, tone: "teal", requiresConnect: true },
    ],
  },
  admin: {
    id: "admin",
    label: "Admin",
    description: "Server",
    icon: ServerCog,
    tone: "blue",
    defaultPath: "/admin/runtime",
    items: [
      { path: "/admin/runtime", label: "Runtime", layout: "collection", icon: Gauge, tone: "teal" },
      // Shown only to an admin on a server that holds accounts.
      { path: "/admin/setup", label: "Setup", layout: "collection", icon: Settings2, tone: "amber", requiresManage: true },
      // The two views an API service has: a place to try queries, and the
      // reference for the endpoints it serves.
      { path: "/admin/workbench", label: "Workbench", layout: "canvas", icon: SquareTerminal, tone: "warm", apiService: true },
      { path: "/admin/catalog", label: "Catalog", layout: "collection", icon: Library, tone: "blue" },
      { path: "/admin/workflows", label: "Workflows", layout: "collection", icon: Workflow, tone: "violet" },
      // Shown only when the server serves its security report. A new install
      // does not: security.read is off by default on an agent server.
      { path: "/admin/security", label: "Security", layout: "collection", icon: ShieldCheck, tone: "warm", requiresCapability: "gj_security" },
      { path: "/admin/code", label: "Code", layout: "document", icon: Code2, tone: "blue" },
      { path: "/admin/config", label: "Config", layout: "document", icon: FileCog, tone: "amber" },
      { path: "/admin/api", label: "API", layout: "document", icon: Braces, tone: "violet", apiService: true },
    ],
  },
  trainer: {
    id: "trainer",
    label: "Trainer",
    description: "Evals",
    icon: GraduationCap,
    tone: "amber",
    defaultPath: "/trainer/reports",
    items: [
      { path: "/trainer/reports", label: "Reports", layout: "document", icon: FlaskConical, tone: "amber" },
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
