# GraphJin MCP Claude Plugin

This plugin registers a GraphJin MCP server in Claude Code:

- command: `deeporg`
- args: `mcp --path ${GRAPHJIN_CONFIG_PATH:-./config}`

## Notes

- `GRAPHJIN_CONFIG_PATH` is optional. Default is `./config`.
- `deeporg` must be available in your `PATH`. Releases before v3.21.22 installed only `graphjin`; update to get `deeporg`.
