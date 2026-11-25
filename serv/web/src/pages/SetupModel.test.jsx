import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import SetupModel from "./SetupModel";

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
    if (reply === "down") {
      throw new TypeError("Failed to fetch");
    }
    return { ok: reply.status >= 200 && reply.status < 300, status: reply.status, text: async () => JSON.stringify(reply.body) };
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("setup: choose a model", () => {
  it("sends the provider and key and waits for the agent to start", async () => {
    const fetchMock = stubFetch([
      { status: 200, body: { provider: "anthropic", restarting: true } },
      "down",
      { status: 200, body: { needs_model: false } },
    ]);
    const onDone = vi.fn();
    render(<SetupModel onDone={onDone} />);

    expect(screen.getByRole("heading", { name: "Choose a model" })).toBeInTheDocument();
    const save = screen.getByRole("button", { name: "Check and save" });
    expect(save).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "anthropic" } });
    fireEvent.change(screen.getByLabelText(/^API key/), { target: { value: " sk-test-key " } });
    fireEvent.click(save);

    expect(await screen.findByRole("status")).toHaveTextContent("The key works. Starting the agent");
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1), { timeout: 5000 });
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/setup/model");
    // No model is named unless the admin asks to name one.
    expect(JSON.parse(options.body)).toEqual({ provider: "anthropic", api_key: "sk-test-key", model: "" });
  });

  it("shows why a key was refused and lets it be tried again", async () => {
    stubFetch([{ status: 400, body: { error: "The provider did not accept that key. Check that it was copied in full and belongs to this provider." } }]);
    const onDone = vi.fn();
    render(<SetupModel onDone={onDone} />);
    fireEvent.change(screen.getByLabelText(/^API key/), { target: { value: "sk-wrong" } });
    fireEvent.click(screen.getByRole("button", { name: "Check and save" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The provider did not accept that key");
    expect(onDone).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Check and save" })).toBeEnabled();
  });

  it("sends a model name only when one is given", async () => {
    const fetchMock = stubFetch([{ status: 200, body: {} }, { status: 200, body: { needs_model: false } }]);
    const onDone = vi.fn();
    render(<SetupModel onDone={onDone} />);
    fireEvent.click(screen.getByRole("button", { name: "Name a specific model" }));
    fireEvent.change(screen.getByLabelText(/^Model name/), { target: { value: "some-model" } });
    fireEvent.change(screen.getByLabelText(/^API key/), { target: { value: "sk-test-key" } });
    fireEvent.click(screen.getByRole("button", { name: "Check and save" }));

    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1), { timeout: 5000 });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ provider: "openai", api_key: "sk-test-key", model: "some-model" });
  });

  it("can be left for later, and says what stays off", () => {
    stubFetch([{ status: 200, body: {} }]);
    const onSkip = vi.fn();
    render(<SetupModel onSkip={onSkip} />);
    expect(screen.getByText(/the built-in agent and routines stay off/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Do this later" }));
    expect(onSkip).toHaveBeenCalledTimes(1);
  });
});
