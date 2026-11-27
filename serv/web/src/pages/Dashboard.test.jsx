import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Dashboard from "./Dashboard";
import SecurityAudit from "./SecurityAudit";
import { ConsoleProvider } from "@/components/Layout/console-context";
import { graphqlRequest } from "../services/graphql";

vi.mock("../services/graphql", async (original) => ({
  ...(await original()),
  graphqlRequest: vi.fn(),
}));

afterEach(cleanup);

function renderWith(capabilities, page = <Dashboard />) {
  const bootstrap = { workspaces: [{ id: "user", capabilities: [] }, { id: "admin", capabilities }] };
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ConsoleProvider value={{ bootstrap, isLoading: false, error: null }}>
        <MemoryRouter>{page}</MemoryRouter>
      </ConsoleProvider>
    </QueryClientProvider>
  );
}

function asked(root) {
  return graphqlRequest.mock.calls.some(([query]) => query.includes(root));
}

describe("the Runtime page", () => {
  beforeEach(() => {
    graphqlRequest.mockReset();
    graphqlRequest.mockImplementation(async (query) => {
      if (query.includes("gj_security")) {
        return { data: { summary: null, findings: [{ id: "f1", severity: "high", title: "Raw queries are on" }] } };
      }
      if (query.includes("gj_runtime")) {
        return { data: { status: [{ id: "status", status: "ready", summary: "DeepORG runtime is ready." }], sources: [], events: [] } };
      }
      return { data: { capabilities: [], tables: [], relationships: [] } };
    });
  });

  it("does not ask a new install for a security report it does not serve", async () => {
    renderWith(["gj_catalog", "gj_config", "gj_runtime"]);

    expect(await screen.findByText("DeepORG runtime is ready.")).toBeInTheDocument();
    expect(await screen.findByText("No sources")).toBeInTheDocument();
    expect(asked("gj_runtime")).toBe(true);
    expect(asked("gj_security")).toBe(false);
    expect(screen.queryByText("Security")).not.toBeInTheDocument();
    expect(screen.queryByText("All findings")).not.toBeInTheDocument();
  });

  it("shows the findings where the server serves them", async () => {
    renderWith(["gj_catalog", "gj_runtime", "gj_security"]);

    expect(await screen.findByText("Raw queries are on")).toBeInTheDocument();
    expect(screen.getByText("Security")).toBeInTheDocument();
    expect(asked("gj_security")).toBe(true);
  });
});

describe("the Security page", () => {
  beforeEach(() => {
    graphqlRequest.mockReset();
    graphqlRequest.mockResolvedValue({ data: { summary: { mode: "dev" }, findings: [], policies: [] } });
  });

  it("says there is no report on a server that does not serve one", async () => {
    renderWith(["gj_catalog", "gj_runtime"], <SecurityAudit />);

    expect(await screen.findByText("No security report")).toBeInTheDocument();
    expect(graphqlRequest).not.toHaveBeenCalled();
  });

  it("asks for the report where it is served", async () => {
    renderWith(["gj_runtime", "gj_security"], <SecurityAudit />);

    expect((await screen.findAllByText("Findings")).length).toBeGreaterThan(0);
    expect(asked("gj_security")).toBe(true);
    expect(screen.queryByText("No security report")).not.toBeInTheDocument();
  });
});
