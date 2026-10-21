# MCP plugin implementation

decision: Implement the user-authorized minimal MCP generator and functioning plugins/mcp/example in the upstream worktree; Mission Control owns application validation. Unit tests remain small boundary checks. No retries, hosted identity service, generic qualification framework, or release controller.
decision: Tool descriptions come from remote function Go documentation and field descriptions from resolved type grammar. Expose only explicitly selected ordinary queries and commands.
decision: Existing official-CLI plugin generation requires a released core without replacements. The development example uses the public Go embedding API with the same generator; release installation continues to use the existing exact-host plugin machinery.
