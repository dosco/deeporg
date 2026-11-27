import React, { Suspense, lazy } from "react";
import { BrowserRouter, Navigate, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import Layout from "./components/Layout/Layout";
import { ThemeProvider } from "./components/theme-provider";
import ErrorBoundary from "./ErrorBoundary";
import { LoadingState } from "./components/ui";
import { useConsole } from "./components/Layout/console-context";
import { homePath } from "./app/workspaces";

import Dashboard from "./pages/Dashboard";
import CatalogExplorer from "./pages/CatalogExplorer";
import SecurityAudit from "./pages/SecurityAudit";
import CodeInsight from "./pages/CodeInsight";
import ConfigViewer from "./pages/ConfigViewer";

const QueryEditor = lazy(() => import("./pages/QueryEditor"));
const ApiDocs = lazy(() => import("./pages/ApiDocs"));
const Inbox = lazy(() => import("./pages/Inbox"));
const AgentChat = lazy(() => import("./pages/AgentChat"));
const MissionControl = lazy(() => import("./pages/MissionControl"));
const Memory = lazy(() => import("./pages/Memory"));
const Skills = lazy(() => import("./pages/Skills"));
const Connect = lazy(() => import("./pages/Connect"));
const AdminSetup = lazy(() => import("./pages/AdminSetup"));
const EvalReports = lazy(() => import("./pages/EvalReports"));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30000,
      retry: 1,
    },
  },
});

const App = () => {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <BrowserRouter basename="/">
            <Layout>
              <Suspense fallback={<LoadingState />}>
                <Routes>
                  <Route path="/" element={<Home />} />
                  <Route path="/setup" element={<SetupLanding />} />
                  {/* Layout shows the approval screen at /connect. This keeps the
                      address, with the agent's request, while the console loads. */}
                  <Route path="/connect" element={<LoadingState />} />
                  <Route path="/invite" element={<LoadingState />} />
                  <Route path="/reset" element={<LoadingState />} />
                  <Route path="/user/agent" element={<AgentChat />} />
                  <Route path="/user/inbox" element={<Inbox />} />
                  <Route path="/user/tasks" element={<MissionControl section="tasks" />} />
                  <Route path="/user/watches" element={<MissionControl section="watches" />} />
                  <Route path="/user/artifacts" element={<MissionControl section="annotations" />} />
                  <Route path="/user/memory" element={<Memory />} />
                  <Route path="/user/skills" element={<Skills />} />
                  <Route path="/user/connect" element={<Connect />} />

                  <Route path="/admin/setup" element={<AdminSetup />} />
                  <Route path="/admin/runtime" element={<Dashboard />} />
                  <Route path="/admin/workbench" element={<QueryEditor />} />
                  <Route path="/admin/catalog" element={<CatalogExplorer />} />
                  <Route path="/admin/workflows" element={<CatalogExplorer initialKind="workflow" title="Workflows" />} />
                  <Route path="/admin/security" element={<SecurityAudit />} />
                  <Route path="/admin/code" element={<CodeInsight />} />
                  <Route path="/admin/config" element={<ConfigViewer />} />
                  <Route path="/admin/api" element={<ApiDocs />} />
                  <Route path="/trainer/reports" element={<EvalReports />} />

                  <Route path="/agent" element={<NavigateWithSearch to="/user/agent" />} />
                  <Route path="/mission" element={<LegacyMissionRedirect />} />
                  <Route path="/workbench" element={<NavigateWithSearch to="/admin/workbench" />} />
                  <Route path="/catalog" element={<NavigateWithSearch to="/admin/catalog" />} />
                  <Route path="/schema" element={<NavigateWithSearch to="/admin/catalog" />} />
                  <Route path="/queries" element={<NavigateWithSearch to="/admin/catalog" />} />
                  <Route path="/security" element={<NavigateWithSearch to="/admin/security" />} />
                  <Route path="/code" element={<NavigateWithSearch to="/admin/code" />} />
                  <Route path="/config" element={<NavigateWithSearch to="/admin/config" />} />
                  <Route path="/databases" element={<NavigateWithSearch to="/admin/runtime" />} />
                  <Route path="/api-docs" element={<NavigateWithSearch to="/admin/api" />} />
                  <Route path="*" element={<Home />} />
                </Routes>
              </Suspense>
            </Layout>
          </BrowserRouter>
        </ThemeProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
};

// Home sends the browser to where this server's console opens. It waits for
// the console to know which kind of server it is talking to.
function Home() {
  const { bootstrap, isLoading } = useConsole();
  if (isLoading) {
    return <LoadingState />;
  }
  return <Navigate to={homePath(bootstrap)} replace />;
}

// SetupLanding holds the setup link's address until the console knows whether
// setup is still needed. While it is, Layout shows the setup screen in place
// of every route; this only runs once it is not, and then the link is spent.
function SetupLanding() {
  const { isLoading } = useConsole();
  if (isLoading) {
    return <LoadingState />;
  }
  return <Navigate to="/user/agent" replace />;
}

function NavigateWithSearch({ to }) {
  const search = window.location.search || "";
  return <Navigate to={`${to}${search}`} replace />;
}

function LegacyMissionRedirect() {
  const params = new URLSearchParams(window.location.search);
  const tab = params.get("tab");
  params.delete("tab");
  const target = tab === "watches" ? "/user/watches" : tab === "annotations" ? "/user/artifacts" : "/user/tasks";
  const search = params.toString();
  return <Navigate to={`${target}${search ? `?${search}` : ""}`} replace />;
}

export default App;
