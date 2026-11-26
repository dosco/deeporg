import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import SetupDatabase from "./SetupDatabase";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
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

describe("setup: connect your data", () => {
  it("sends the connection URL and waits for the server to come back", async () => {
    const fetchMock = stubFetch([
      { status: 200, body: { name: "main", type: "postgres", restarting: true } },
      "down",
      { status: 200, body: { needs_admin: false, needs_database: true } },
      { status: 200, body: { needs_admin: false, needs_database: false, tables: 7 } },
    ]);
    const onDone = vi.fn();
    render(<SetupDatabase onDone={onDone} />);

    expect(screen.getByRole("heading", { name: "Connect your data" })).toBeInTheDocument();
    const connect = screen.getByRole("button", { name: "Connect" });
    expect(connect).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Connection URL"), { target: { value: " postgres://u:p@db.example.com:5432/shop " } });
    fireEvent.click(connect);

    expect(await screen.findByRole("status")).toHaveTextContent("Connected. Starting up with your database");
    // The person is told what was found before moving on.
    expect(await screen.findByRole("heading", { name: "Your data is connected" }, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("We found 7 tables.");
    expect(onDone).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onDone).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/setup/database");
    expect(JSON.parse(options.body)).toEqual({ url: "postgres://u:p@db.example.com:5432/shop" });
  });

  it("shows the server's advice when the connection fails, and nothing more", async () => {
    stubFetch([{ status: 400, body: { error: "We could not find the host db.example.com. Check the host name for typos.", detail: "lookup db.example.com: no such host" } }]);
    const onDone = vi.fn();
    render(<SetupDatabase onDone={onDone} />);
    fireEvent.change(screen.getByLabelText("Connection URL"), { target: { value: "postgres://u:p@db.example.com/shop" } });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("We could not find the host db.example.com");
    expect(onDone).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Connect" })).toBeEnabled();
  });

  it("takes the details one by one for someone without a URL", async () => {
    const fetchMock = stubFetch([
      { status: 200, body: { restarting: true } },
      { status: 200, body: { needs_database: false } },
    ]);
    const onDone = vi.fn();
    render(<SetupDatabase onDone={onDone} />);
    fireEvent.click(screen.getByRole("button", { name: "Enter the details separately" }));
    expect(screen.queryByLabelText("Connection URL")).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Host"), { target: { value: "db.internal" } });
    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Database name"), { target: { value: "shop" } });
    fireEvent.change(screen.getByLabelText("User"), { target: { value: "reader" } });
    fireEvent.change(screen.getByLabelText("Database password"), { target: { value: "s3cret" } });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));

    expect(await screen.findByRole("heading", { name: "Your data is connected" }, { timeout: 5000 })).toBeInTheDocument();
    // An older server does not say how many tables it found.
    expect(screen.getByRole("status")).toHaveTextContent("The connection works.");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onDone).toHaveBeenCalledTimes(1);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ type: "postgres", host: "db.internal", port: 0, database: "shop", user: "reader", password: "s3cret" });
  });

  it("offers sample data only where the server can load it", async () => {
    const fetchMock = stubFetch([
      { status: 200, body: { name: "sample", type: "sqlite", restarting: true } },
      { status: 200, body: { needs_database: false } },
    ]);
    const onDone = vi.fn();
    const { rerender } = render(<SetupDatabase onDone={onDone} />);
    expect(screen.queryByRole("button", { name: "Use sample data" })).not.toBeInTheDocument();

    rerender(<SetupDatabase onDone={onDone} sampleData />);
    fireEvent.click(screen.getByRole("button", { name: "Use sample data" }));
    fireEvent.click(await screen.findByRole("button", { name: "Continue" }, { timeout: 5000 }));
    expect(onDone).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/setup/database");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ sample: true });
  });

  it("says so when the sample data cannot be prepared", async () => {
    stubFetch([{ status: 500, body: { error: "The sample data could not be prepared. Connect your own database, or see the server log for why." } }]);
    const onDone = vi.fn();
    render(<SetupDatabase onDone={onDone} sampleData />);
    fireEvent.click(screen.getByRole("button", { name: "Use sample data" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The sample data could not be prepared");
    expect(onDone).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Use sample data" })).toBeEnabled();
  });

  it("can be left for later", () => {
    stubFetch([{ status: 200, body: {} }]);
    const onSkip = vi.fn();
    render(<SetupDatabase onSkip={onSkip} />);
    fireEvent.click(screen.getByRole("button", { name: "Do this later" }));
    expect(onSkip).toHaveBeenCalledTimes(1);
  });
});
