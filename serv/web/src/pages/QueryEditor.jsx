import "../setupGraphiQLWorkers";
import React, { useMemo } from "react";
import { createGraphiQLFetcher } from "@graphiql/toolkit";
import { GraphiQL } from "graphiql";

import { PageHeader } from "../components/ui";
import { useTheme } from "@/components/theme-provider";
import { useConsole } from "@/components/Layout/console-context";
import { isAPIService } from "@/app/workspaces";
import "graphiql/style.css";

const defaultEndpoint = import.meta.env.VITE_DEFAULT_ENDPOINT || "/api/v1/graphql";

const defaultQuery = `query SourceHealth {
  gj_runtime(where: { kind: { eq: "source" } }, order_by: { source: asc }) {
    source
    source_kind
    status
    severity
    summary
    next_action
  }
}
`;

// An API service has no runtime root to ask about. Its Workbench starts with
// a note on what happens to the queries written here.
const apiServiceQuery = `# Write a query for one of your tables and run it.
#
# Give it a name, as in "query listProducts". While production is false the
# query is saved under that name in the queries folder, and saved again when
# you change it. In production only those saved queries are served.

query listProducts {
  products(limit: 5) {
    id
  }
}
`;

const QueryEditor = () => {
  const { resolvedTheme } = useTheme();
  const { bootstrap } = useConsole();
  const initialQuery = isAPIService(bootstrap) ? apiServiceQuery : defaultQuery;
  const fetcher = useMemo(() => {
    let apiPath = defaultEndpoint;
    const urlParams = new URLSearchParams(window.location.search);
    const ep = urlParams.get("endpoint");

    // Only allow relative paths, reject protocol-relative URLs like //evil.com
    if (ep !== null && ep !== "" && ep.startsWith("/") && !ep.startsWith("//")) {
      apiPath = ep;
    }

    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";

    const fetcher = createGraphiQLFetcher({
      url: `${window.location.protocol}//${window.location.host}${apiPath}`,
      subscriptionUrl: `${wsProtocol}//${window.location.host}${apiPath}`,
    });

    return fetcher;
  }, []);

  return (
    <div className="grid h-[calc(100dvh-var(--chrome-top)-var(--chrome-bottom))] min-h-[38rem] grid-rows-[auto_minmax(0,1fr)] overflow-hidden">
      <div className="px-4 py-4 sm:px-6">
        <PageHeader title="Workbench" />
      </div>
      <div className="graphiql-shell min-h-0">
        <GraphiQL fetcher={fetcher} forcedTheme={resolvedTheme} initialQuery={initialQuery} />
      </div>
    </div>
  );
};

export default QueryEditor;
