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

  it("changes a role, makes a password link and removes a person, but not the admin's own account", async () => {
    let team = [
      { email: "ada@example.com", role: "admin", you: true },
      { email: "sam@example.com", role: "user", you: false },
    ];
    const fetchMock = stubFetch((method, url, options) => {
      if (method === "POST" && url === "/api/v1/setup/people") {
        const body = JSON.parse(options.body);
        team = body.remove ? team.filter((person) => person.email !== body.email) : team.map((person) => (person.email === body.email ? { ...person, role: body.role } : person));
        return { status: 200, body: body.remove ? { removed: true } : { email: body.email, role: body.role } };
      }
      if (method === "POST" && url === "/api/v1/setup/people/reset") {
        return { status: 200, body: { email: "sam@example.com", link: "http://127.0.0.1:8080/reset?token=xyz", expires_at: 1 } };
      }
      return { status: 200, body: { ...details, people: team } };
    });
    renderPage(admin);

    // The admin's own row says so and offers nothing that would lock them out.
    expect(await screen.findByText("admin, you")).toBeInTheDocument();
    expect(screen.queryByLabelText("What ada@example.com can do")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove ada@example.com" })).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("What sam@example.com can do"), { target: { value: "admin" } });
    await waitFor(() => expect(screen.getByLabelText("What sam@example.com can do")).toHaveValue("admin"));
    const sent = () => fetchMock.mock.calls.filter(([, options]) => options?.method === "POST").map(([url, options]) => [url, JSON.parse(options.body)]);
    expect(sent()[0]).toEqual(["/api/v1/setup/people", { email: "sam@example.com", role: "admin" }]);

    fireEvent.click(screen.getByRole("button", { name: "New password link for sam@example.com" }));
    expect(await screen.findByLabelText("Password link")).toHaveTextContent("http://127.0.0.1:8080/reset?token=xyz");
    expect(screen.getByRole("status")).toHaveTextContent("It works once, for 24 hours.");

    // Removing asks first, and backing out changes nothing.
    fireEvent.click(screen.getByRole("button", { name: "Remove sam@example.com" }));
    fireEvent.click(screen.getByRole("button", { name: "Keep" }));
    expect(sent().some(([, body]) => body.remove)).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Remove sam@example.com" }));
    expect(screen.getByText("Remove them? They are signed out at once.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Remove sam@example.com" }));
    await waitFor(() => expect(screen.queryByText("sam@example.com")).not.toBeInTheDocument());
    expect(sent().at(-1)).toEqual(["/api/v1/setup/people", { email: "sam@example.com", remove: true }]);
    // The link for someone who is gone is not left on the page.
    expect(screen.queryByLabelText("Password link")).not.toBeInTheDocument();
  });

  it("says why when a change is refused", async () => {
    stubFetch((method, url) => {
      if (method === "POST" && url === "/api/v1/setup/people") {
        return { status: 409, body: { error: "this is the only admin; make someone else an admin first" } };
      }
      return { status: 200, body: { ...details, people: [{ email: "eve@example.com", role: "admin", you: false }] } };
    });
    renderPage(admin);

    fireEvent.change(await screen.findByLabelText("What eve@example.com can do"), { target: { value: "user" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("this is the only admin");
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
