import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import Invite from "./Invite";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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

describe("invite: join by link", () => {
  it("says who the invite is for and asks only for a password", async () => {
    const fetchMock = stubFetch([
      { status: 200, body: { email: "sam@example.com" } },
      { status: 200, body: { email: "sam@example.com", role: "user" } },
    ]);
    const onDone = vi.fn();
    render(<Invite search="?token=abc123" onDone={onDone} />);

    expect(await screen.findByText("sam@example.com")).toBeInTheDocument();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/setup/invite?token=abc123");
    const create = screen.getByRole("button", { name: "Create account" });
    expect(create).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/^Password/), { target: { value: "a long enough password" } });
    fireEvent.click(create);

    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    const [url, options] = fetchMock.mock.calls[1];
    expect(url).toBe("/api/v1/setup/invite");
    expect(JSON.parse(options.body)).toEqual({ token: "abc123", password: "a long enough password" });
  });

  it("says so when the link no longer works", async () => {
    stubFetch([{ status: 404, body: { error: "This invite link is not valid any more. Ask whoever invited you for a new one." } }]);
    render(<Invite search="?token=old" onDone={vi.fn()} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("This invite link is not valid any more.");
    expect(screen.queryByLabelText(/^Password/)).not.toBeInTheDocument();
  });

  it("keeps the form when the password is refused", async () => {
    stubFetch([
      { status: 200, body: { email: "sam@example.com" } },
      { status: 400, body: { error: "choose a password of at least 10 characters" } },
    ]);
    const onDone = vi.fn();
    render(<Invite search="?token=abc123" onDone={onDone} />);

    fireEvent.change(await screen.findByLabelText(/^Password/), { target: { value: "1234567890" } });
    fireEvent.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("at least 10 characters");
    expect(onDone).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Create account" })).toBeEnabled();
  });

  it("explains a link with no invite in it", () => {
    const fetchMock = stubFetch([{ status: 200, body: {} }]);
    render(<Invite search="" onDone={vi.fn()} />);
    expect(screen.getByRole("alert")).toHaveTextContent("This link has no invite in it.");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
