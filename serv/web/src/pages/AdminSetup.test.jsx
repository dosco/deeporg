import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import AdminSetup from "./AdminSetup";
import { ConsoleProvider } from "@/components/Layout/console-context";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const details = {
  store_backed: true,
  databases: [{ name: "sample", type: "sqlite", tables: 7 }],
  model: { provider: "openai", model: "", ready: true },
  people: [{ email: "ada@example.com", role: "admin" }],
  invites: [],
};

// answer picks a reply by method and path, so the order of requests does not matter.
function stubFetch(answer) {
  const fetchMock = vi.fn(async (url, options = {}) => {
    const reply = answer(options.method || "GET", url, options);
    return { ok: reply.status >= 200 && reply.status < 300, status: reply.status, text: async () => JSON.stringify(reply.body) };
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderPage(bootstrap) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ConsoleProvider value={{ bootstrap, isLoading: false, error: null }}>
        <AdminSetup />
      </ConsoleProvider>
    </QueryClientProvider>
  );
}

const admin = { identity: { role: "admin" }, manage: { store_backed: true } };

describe("admin setup", () => {
  it("shows what is connected and who can sign in", async () => {
    stubFetch(() => ({ status: 200, body: details }));
    renderPage(admin);

    expect(await screen.findByText("sample")).toBeInTheDocument();
    expect(screen.getByText("sqlite, 7 tables")).toBeInTheDocument();
    expect(screen.getByText(/The built-in agent uses/)).toHaveTextContent("openai, with the provider's default model.");
    expect(screen.getByText("ada@example.com")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect another database" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Change the model" })).toBeInTheDocument();
  });

  it("opens the database screen with a name to give the new database", async () => {
    stubFetch(() => ({ status: 200, body: details }));
    renderPage(admin);

    fireEvent.click(await screen.findByRole("button", { name: "Connect another database" }));
    expect(screen.getByRole("heading", { name: "Connect another database" })).toBeInTheDocument();
    expect(screen.getByLabelText(/^Name/)).toBeInTheDocument();
    // Leaving goes back to the page, with nothing changed.
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("heading", { name: "Setup" })).toBeInTheDocument();
  });

  it("makes an invite link to send, and can withdraw it", async () => {
    let invited = false;
    const fetchMock = stubFetch((method, url) => {
      if (method === "POST" && url === "/api/v1/setup/invites") {
        invited = true;
        return { status: 200, body: { email: "sam@example.com", link: "http://127.0.0.1:8080/invite?token=abc", expires_at: 1 } };
      }
      if (method === "DELETE") {
        invited = false;
        return { status: 200, body: { withdrawn: true } };
      }
      return { status: 200, body: { ...details, invites: invited ? [{ email: "sam@example.com", role: "user" }] : [] } };
    });
    renderPage(admin);

    fireEvent.change(await screen.findByLabelText("Invite a teammate"), { target: { value: " sam@example.com " } });
    fireEvent.click(screen.getByRole("button", { name: "Make an invite link" }));

    expect(await screen.findByLabelText("Invite link")).toHaveTextContent("http://127.0.0.1:8080/invite?token=abc");
    expect(screen.getByRole("status")).toHaveTextContent("Send this link to sam@example.com. It works once, for a week.");
    const post = fetchMock.mock.calls.find(([, options]) => options?.method === "POST");
    expect(JSON.parse(post[1].body)).toEqual({ email: "sam@example.com", role: "user" });

    fireEvent.click(await screen.findByRole("button", { name: "Withdraw" }));
    await waitFor(() => expect(screen.queryByLabelText("Invite link")).not.toBeInTheDocument());
    const withdrawn = fetchMock.mock.calls.find(([, options]) => options?.method === "DELETE");
    expect(withdrawn[0]).toBe("/api/v1/setup/invites?email=sam%40example.com");
  });

  it("shows only people on a server whose data and model are set in config files", async () => {
    stubFetch(() => ({ status: 200, body: { ...details, store_backed: false } }));
    renderPage({ identity: { role: "admin" }, manage: { store_backed: false } });

    expect(await screen.findByText("ada@example.com")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect another database" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Change the model" })).not.toBeInTheDocument();
  });

  it("is closed to anyone who is not an admin with an account", () => {
    const fetchMock = stubFetch(() => ({ status: 200, body: details }));
    renderPage({ identity: { role: "user" } });

    expect(screen.getByText(/Only an admin signed in with a DeepORG account/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
