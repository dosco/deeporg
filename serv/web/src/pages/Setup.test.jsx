import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import Setup from "./Setup";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubFetch(status, body) {
  const fetchMock = vi.fn(async () => ({
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
  }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function fill(label, value) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

describe("setup", () => {
  it("creates the first admin with the token from the setup link", async () => {
    const fetchMock = stubFetch(200, { email: "ada@example.com", role: "admin" });
    const onDone = vi.fn();
    render(<Setup search="?token=link-token" onDone={onDone} />);

    expect(screen.getByRole("heading", { name: "Create your admin account" })).toBeInTheDocument();
    // The link already carries the token, so the newcomer is not asked for it.
    expect(screen.queryByLabelText(/Setup token/)).not.toBeInTheDocument();

    const create = screen.getByRole("button", { name: "Create account" });
    expect(create).toBeDisabled();
    fill("Email", "ada@example.com");
    fill(/^Password/, "too short");
    expect(create).toBeDisabled();
    fill(/^Password/, "a long enough password");
    expect(create).toBeEnabled();
    fireEvent.click(create);

    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/setup/admin");
    expect(JSON.parse(options.body)).toEqual({ token: "link-token", email: "ada@example.com", password: "a long enough password" });
  });

  it("keeps the link's token when the address changes under it", () => {
    stubFetch(200, {});
    const { rerender } = render(<Setup search="?token=link-token" />);
    rerender(<Setup search="" />);
    expect(screen.queryByLabelText(/Setup token/)).not.toBeInTheDocument();
  });

  it("asks for the token when the address has none", () => {
    stubFetch(200, {});
    render(<Setup search="" />);
    expect(screen.getByLabelText(/Setup token/)).toBeInTheDocument();
    fill("Email", "ada@example.com");
    fill(/^Password/, "a long enough password");
    expect(screen.getByRole("button", { name: "Create account" })).toBeDisabled();
    fill(/Setup token/, "typed-token");
    expect(screen.getByRole("button", { name: "Create account" })).toBeEnabled();
  });

  it("shows the server's reason when the account is refused", async () => {
    stubFetch(403, { error: "the setup link is not valid; use the one this server printed when it started" });
    const onDone = vi.fn();
    render(<Setup search="?token=stale" onDone={onDone} />);
    fill("Email", "ada@example.com");
    fill(/^Password/, "a long enough password");
    fireEvent.click(screen.getByRole("button", { name: "Create account" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("the setup link is not valid");
    expect(onDone).not.toHaveBeenCalled();
    // The form is usable again after a refusal.
    expect(screen.getByRole("button", { name: "Create account" })).toBeEnabled();
  });
});
