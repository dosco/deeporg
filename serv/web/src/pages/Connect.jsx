import React from "react";

import { useConsole } from "@/components/Layout/console-context";
import { Button } from "@/components/ui/button";
import { connectState, returnToModelStep } from "@/services/session";

// The name the agent lists this server under.
const SERVER_NAME = "deeporg";

export function claudeCodeCommand(mcpURL) {
  return `claude mcp add --transport http --scope user ${SERVER_NAME} ${mcpURL}`;
}

// Connect tells a signed-in person how to connect their own agent. The
// command carries the server's address and no secret: the agent sends them
// back here to sign in and allow it.
const Connect = () => {
  const { bootstrap } = useConsole();
  const { mcpURL, answers, isAdmin } = connectState(bootstrap);

  if (!mcpURL) {
    return (
      <div className="max-w-2xl">
        <h1 className="text-2xl font-bold tracking-tight">Connect your own agent</h1>
        <p className="mt-3 text-sm text-muted-foreground">This server does not sign agents in with a DeepORG account, so there is nothing to set up here.</p>
      </div>
    );
  }

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-bold tracking-tight">Connect your own agent</h1>
      <p className="mt-3 text-sm text-muted-foreground">
        {answers === "queries"
          ? "An agent such as Claude Code can look up your tables and run queries through DeepORG."
          : "An agent such as Claude Code can ask DeepORG questions about your data."}
        {" "}It signs in as you, so it sees what you can see.
      </p>
      {!answers && (
        <div className="mt-6 grid gap-3 text-sm">
          <p>
            A connected agent gets its answers from DeepORG's built-in agent, and that has no model yet. You can connect now, and it will be able to answer once a model is chosen.
          </p>
          {isAdmin ? (
            <div>
              <Button type="button" variant="secondary" onClick={() => returnToModelStep()}>Choose a model</Button>
            </div>
          ) : (
            <p className="text-muted-foreground">Ask your admin to choose one.</p>
          )}
        </div>
      )}

      <h2 className="mt-10 text-base font-bold">Claude Code</h2>
      <ol className="mt-4 grid gap-6 text-sm">
        <li className="grid gap-2">
          <span><span className="font-mono text-muted-foreground">1</span> &nbsp;Run this in a terminal.</span>
          <Copyable label="Command for Claude Code" text={claudeCodeCommand(mcpURL)} />
        </li>
        <li>
          <span className="font-mono text-muted-foreground">2</span> &nbsp;Start Claude Code and type <span className="font-mono">/mcp</span>. Choose <span className="font-mono">{SERVER_NAME}</span>, then Authenticate.
        </li>
        <li>
          <span className="font-mono text-muted-foreground">3</span> &nbsp;Your browser opens on this server. Choose Allow, and Claude Code is connected.
        </li>
      </ol>

      <h2 className="mt-10 text-base font-bold">Other agents</h2>
      <p className="mt-3 text-sm text-muted-foreground">
        Other agents that support MCP sign-in can try this address. They send you here to allow them.
      </p>
      <div className="mt-3">
        <Copyable label="Address for other agents" text={mcpURL} />
      </div>
    </div>
  );
};

// Copyable shows a line to copy and a way to copy it.
function Copyable({ label, text }) {
  const [copied, setCopied] = React.useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Without clipboard access the text is still there to select.
    }
  }

  return (
    <div className="flex items-start gap-4">
      <code aria-label={label} className="min-w-0 flex-1 select-all break-all font-mono text-[0.8125rem] leading-6 text-foreground">{text}</code>
      <button type="button" className="shrink-0 text-sm text-muted-foreground underline underline-offset-4 hover:text-foreground" onClick={copy}>
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  );
}

export default Connect;
