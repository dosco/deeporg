import React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";

import Header from "./Header";
import { ConsoleProvider } from "./console-context";
import { routeDefinition } from "@/app/workspaces";
import { cn } from "@/lib/utils";
import { fetchConsoleBootstrap } from "@/services/console";
import { useNotificationStream } from "@/services/notifications";
import { adoptSuggestedIdentity, operatorIdentityKey, useOperatorIdentity } from "@/services/identity";
import { setupState } from "@/services/session";
import Setup from "@/pages/Setup";
import SetupDatabase from "@/pages/SetupDatabase";
import SetupModel from "@/pages/SetupModel";

// The admin can leave a setup step for later. That choice lasts for this
// browser tab; the step comes back the next time the console is opened.
const SKIP_DATABASE_KEY = "deeporg.setup.skipDatabase";
const SKIP_MODEL_KEY = "deeporg.setup.skipModel";

function stepSkipped(key) {
  try {
    return sessionStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

function rememberSkip(key) {
  try {
    sessionStorage.setItem(key, "1");
  } catch {
    // Private windows may refuse storage; skipping still works for this page.
  }
}

const layoutClasses = {
  canvas: "min-h-0 overflow-hidden",
  collection: "w-full px-4 py-5 sm:px-6 lg:py-6",
  document: "w-full px-4 py-5 sm:px-6 lg:py-6",
};

const Layout = ({ children }) => {
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const identity = useOperatorIdentity();
  const [skippedDatabase, setSkippedDatabase] = React.useState(() => stepSkipped(SKIP_DATABASE_KEY));
  const [skippedModel, setSkippedModel] = React.useState(() => stepSkipped(SKIP_MODEL_KEY));
  const identityKey = operatorIdentityKey(identity);
  const bootstrapQuery = useQuery({
    queryKey: ["console-bootstrap", identityKey],
    queryFn: fetchConsoleBootstrap,
    staleTime: 30000,
    retry: 1,
  });
  // Adopting rewrites identityKey, which refetches bootstrap with the new
  // headers; the suggestion is then absent and this settles after one pass.
  React.useEffect(() => {
    adoptSuggestedIdentity(bootstrapQuery.data?.identity?.suggested);
  }, [bootstrapQuery.data]);
  const notificationsOn = (bootstrapQuery.data?.workspaces || []).some((workspace) => (workspace?.capabilities || []).includes("gj_notification"));
  useNotificationStream(notificationsOn);
  const route = routeDefinition(location.pathname);
  const consoleValue = React.useMemo(() => ({
    bootstrap: bootstrapQuery.data,
    isLoading: bootstrapQuery.isLoading,
    error: bootstrapQuery.error,
  }), [bootstrapQuery.data, bootstrapQuery.error, bootstrapQuery.isLoading]);

  // A new server has no admin yet. Nothing else in the console can work until
  // one exists, so setup takes the whole screen whatever the address was.
  if (setupState(bootstrapQuery.data).needsAdmin) {
    return (
      <div className="relative min-h-dvh bg-background text-foreground">
        <div className="horizon" aria-hidden="true" />
        <div className="starfield" aria-hidden="true" />
        <Setup
          search={location.search}
          onDone={() => {
            // The account exists and this browser holds its session. Drop the
            // one-time token from the address and start again as that admin.
            queryClient.clear();
            navigate("/user/agent", { replace: true });
            void bootstrapQuery.refetch();
          }}
        />
      </div>
    );
  }

  // With an admin in place the next step is the database. Only the admin can
  // take it, so anyone else, signed in or not, gets the ordinary console.
  const signedInAdmin = Boolean(bootstrapQuery.data?.identity?.authenticated) && bootstrapQuery.data?.identity?.role === "admin";
  if (setupState(bootstrapQuery.data).needsDatabase && signedInAdmin && !skippedDatabase) {
    return (
      <div className="relative min-h-dvh bg-background text-foreground">
        <div className="horizon" aria-hidden="true" />
        <div className="starfield" aria-hidden="true" />
        <SetupDatabase
          onDone={() => {
            queryClient.clear();
            navigate("/user/agent", { replace: true });
            void bootstrapQuery.refetch();
          }}
          onSkip={() => {
            rememberSkip(SKIP_DATABASE_KEY);
            setSkippedDatabase(true);
          }}
        />
      </div>
    );
  }

  // Last, a model for the built-in agent.
  if (setupState(bootstrapQuery.data).needsModel && signedInAdmin && !skippedModel) {
    return (
      <div className="relative min-h-dvh bg-background text-foreground">
        <div className="horizon" aria-hidden="true" />
        <div className="starfield" aria-hidden="true" />
        <SetupModel
          onDone={() => {
            queryClient.clear();
            navigate("/user/agent", { replace: true });
            void bootstrapQuery.refetch();
          }}
          onSkip={() => {
            rememberSkip(SKIP_MODEL_KEY);
            setSkippedModel(true);
          }}
        />
      </div>
    );
  }

  return (
    <ConsoleProvider value={consoleValue}>
      <div className="relative min-h-dvh bg-background text-foreground">
        <div className="horizon" aria-hidden="true" />
        <div className="starfield" aria-hidden="true" />
        <a href="#main-content" className="sr-only z-[100] rounded-md bg-foreground px-4 py-2 font-semibold text-background focus:not-sr-only focus:fixed focus:left-3 focus:top-3">
          Skip to content
        </a>
        <div className="relative mx-auto flex min-h-dvh w-full max-w-[100rem] flex-col md:flex-row">
          <Header />
          <main
            id="main-content"
            data-layout={route.layout}
            className={cn(
              "relative min-w-0 flex-1 pb-[var(--chrome-bottom)] md:pb-0",
              layoutClasses[route.layout] || layoutClasses.collection
            )}
          >
            {children}
          </main>
        </div>
      </div>
    </ConsoleProvider>
  );
};

export default Layout;
