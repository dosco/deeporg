import React from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import Connect, { claudeCodeCommand } from "./Connect";
import { ConsoleProvider } from "@/components/Layout/console-context";

afterEach(cleanup);

function renderWith(bootstrap) {
  return render(
    <ConsoleProvider value={{ bootstrap, isLoading: false, error: null }}>
      <Connect />
    </ConsoleProvider>
  );
}

describe("connect your own agent", () => {
  it("gives a command with the server's address and no secret in it", () => {
    renderWith({ connect: { mcp_url: "http://127.0.0.1:8080/api/v1/mcp", answers: "agent" } });

    const command = screen.getByLabelText("Command for Claude Code");
    expect(command).toHaveTextContent("claude mcp add --transport http --scope user deeporg http://127.0.0.1:8080/api/v1/mcp");
    expect(command.textContent).not.toMatch(/Bearer|token|header/i);
    expect(screen.getByLabelText("Address for other agents")).toHaveTextContent("http://127.0.0.1:8080/api/v1/mcp");
    expect(screen.getByText(/Choose Allow, and Claude Code is connected/)).toBeInTheDocument();
  });

  it("puts every option before the name, as Claude Code requires", () => {
    const parts = claudeCodeCommand("https://data.example.com/api/v1/mcp").split(" ");
    expect(parts.slice(-2)).toEqual(["deeporg", "https://data.example.com/api/v1/mcp"]);
  });

  it("says what is missing when the built-in agent has no model yet", () => {
    renderWith({ identity: { role: "admin" }, connect: { mcp_url: "http://127.0.0.1:8080/api/v1/mcp", answers: "" } });
    expect(screen.getByText(/gets its answers from DeepORG's built-in agent, and that has no model yet/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Choose a model" })).toBeInTheDocument();
    // The command is still there: connecting first is fine.
    expect(screen.getByLabelText("Command for Claude Code")).toBeInTheDocument();
  });

  it("sends someone who is not the admin to their admin for the model", () => {
    renderWith({ identity: { role: "user" }, connect: { mcp_url: "http://127.0.0.1:8080/api/v1/mcp", answers: "" } });
    expect(screen.getByText("Ask your admin to choose one.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Choose a model" })).not.toBeInTheDocument();
  });

  it("says nothing is missing once the built-in agent can answer", () => {
    renderWith({ identity: { role: "admin" }, connect: { mcp_url: "http://127.0.0.1:8080/api/v1/mcp", answers: "agent" } });
    expect(screen.getByText(/can ask DeepORG questions about your data/)).toBeInTheDocument();
    expect(screen.queryByText(/has no model yet/)).not.toBeInTheDocument();
  });

  it("says so on a server that does not sign agents in with an account", () => {
    renderWith({});
    expect(screen.getByText(/does not sign agents in with a DeepORG account/)).toBeInTheDocument();
    expect(screen.queryByLabelText("Command for Claude Code")).not.toBeInTheDocument();
  });
});
