import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import ConnectApprove from "./ConnectApprove";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// replies answers each request in order; the last reply repeats.
function stubFetch(replies) {
  let call = 0;
  const fetchMock = vi.fn(async () => {
    const reply = replies[Math.min(call, replies.length - 1)];
    call += 1;
    return { ok: reply.status >= 200 && reply.status < 300, status: reply.status, text: async () => JSON.stringify(reply.body) };
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const signedIn = { required: true, signedIn: true, name: "ada@example.com", role: "admin", methods: [] };
const signedOut = { required: true, signedIn: false, name: "", role: "", methods: ["password"] };
const local = { client_name: "Claude Code", destination: "localhost:53124", on_this_computer: true, email: "ada@example.com", role: "admin" };

describe("connect: allow an agent", () => {
  it("shows who is asking and sends the browser back with the answer", async () => {
    const fetchMock = stubFetch([
      { status: 200, body: local },
      { status: 200, body: { redirect: "http://localhost:53124/callback?code=abc&state=xyz" } },
    ]);
    const leave = vi.fn();
    render(<ConnectApprove request="req-1" session={signedIn} leave={leave} />);

    expect(await screen.findByRole("heading", { name: "Allow Claude Code to connect?" })).toBeInTheDocument();
    expect(screen.getByText("ada@example.com")).toBeInTheDocument();
    expect(screen.getByText("It is an app on this computer.")).toBeInTheDocument();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/oauth/consent?request=req-1");

    fireEvent.click(screen.getByRole("button", { name: "Allow" }));
    await waitFor(() => expect(leave).toHaveBeenCalledWith("http://localhost:53124/callback?code=abc&state=xyz"));
    const [url, options] = fetchMock.mock.calls[1];
    expect(url).toBe("/api/v1/oauth/consent");
    expect(JSON.parse(options.body)).toEqual({ request: "req-1", allow: true });
    expect(screen.getByRole("heading", { name: "Connected" })).toBeInTheDocument();
  });

  it("names the site when the agent is not on this computer", async () => {
    stubFetch([{ status: 200, body: { ...local, destination: "agent.example.com", on_this_computer: false } }]);
    render(<ConnectApprove request="req-1" session={signedIn} leave={vi.fn()} />);

    expect(await screen.findByText("agent.example.com")).toBeInTheDocument();
    expect(screen.getByText(/Allow this only if you started it yourself/)).toBeInTheDocument();
    expect(screen.queryByText("It is an app on this computer.")).not.toBeInTheDocument();
  });

  it("sends a refusal too, so the agent stops waiting", async () => {
    const fetchMock = stubFetch([
      { status: 200, body: local },
      { status: 200, body: { redirect: "http://localhost:53124/callback?error=access_denied" } },
    ]);
    const leave = vi.fn();
    render(<ConnectApprove request="req-1" session={signedIn} leave={leave} />);

    fireEvent.click(await screen.findByRole("button", { name: "Don't allow" }));
    await waitFor(() => expect(leave).toHaveBeenCalledWith("http://localhost:53124/callback?error=access_denied"));
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ request: "req-1", allow: false });
    expect(screen.getByRole("heading", { name: "Not connected" })).toBeInTheDocument();
  });

  it("asks a signed-out person to sign in before showing anything", () => {
    const fetchMock = stubFetch([{ status: 200, body: local }]);
    render(<ConnectApprove request="req-1" session={signedOut} leave={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "Sign in to connect your app" })).toBeInTheDocument();
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Allow" })).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("says so when the request has expired", async () => {
    stubFetch([{ status: 404, body: { error: "This request has expired or was already answered. Start again from the app you are connecting." } }]);
    render(<ConnectApprove request="old" session={signedIn} leave={vi.fn()} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("This request has expired or was already answered.");
    expect(screen.queryByRole("button", { name: "Allow" })).not.toBeInTheDocument();
  });

  it("explains an address with no request in it", () => {
    render(<ConnectApprove request="" session={signedIn} leave={vi.fn()} />);
    expect(screen.getByRole("heading", { name: "Nothing is waiting to connect" })).toBeInTheDocument();
  });
});
