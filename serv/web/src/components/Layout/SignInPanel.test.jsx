import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import SignInPanel from "./SignInPanel";

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

const signedOut = (methods) => ({ required: true, signedIn: false, name: "", role: "", methods, loginURL: "" });

describe("sign-in panel", () => {
  it("signs in with an email and password, and keeps the token form out of the way", async () => {
    const fetchMock = stubFetch(200, { user_id: "abc" });
    const onChange = vi.fn();
    render(<SignInPanel state={signedOut(["password", "token"])} onChange={onChange} />);

    expect(screen.queryByLabelText("Access token")).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "ada@example.com" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "a long enough password" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => expect(onChange).toHaveBeenCalledTimes(1));
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/auth/session");
    expect(JSON.parse(options.body)).toEqual({ method: "password", email: "ada@example.com", password: "a long enough password" });

    fireEvent.click(screen.getByRole("button", { name: "Use an access token instead" }));
    expect(screen.getByLabelText("Access token")).toBeInTheDocument();
  });

  it("shows why a sign-in was refused", async () => {
    stubFetch(401, { error: "that email and password do not match an account" });
    const onChange = vi.fn();
    render(<SignInPanel state={signedOut(["password", "token"])} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "ada@example.com" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "wrong" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("do not match an account");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("still shows the token form on a server without accounts", () => {
    render(<SignInPanel state={signedOut(["token"])} />);
    expect(screen.getByLabelText("Access token")).toBeInTheDocument();
    expect(screen.queryByLabelText("Email")).not.toBeInTheDocument();
  });
});
