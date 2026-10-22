# MCP plugin implementation

decision: Implement the user-authorized minimal MCP generator and functioning plugins/mcp/example in the upstream worktree. Unit tests remain small boundary checks. No retries, hosted identity service, generic qualification framework, or release controller.
decision: Tool descriptions come from remote function Go documentation and field descriptions from resolved type grammar. Expose only explicitly selected ordinary queries and commands.
decision: Existing official-CLI plugin generation requires a released core without replacements. The development example uses the public Go embedding API with the same generator; release installation continues to use the existing exact-host plugin machinery.
correction: Independent review found mcp-go's default annotations falsely marked every tool destructive. Clear unclaimed hints rather than inferring purity from query declarations; unit-check the serialized metadata.
correction: Keep HTTP transport errors neutral; only the MCP command bridge supplies the uncertainty warning. Schema output now includes only reachable definitions and preserves an unwrapped input type's documentation.
