// Package clipublish marks application remotes for CLI and MCP generation. It has no
// runtime dependencies and never registers or invokes application functions.
package clipublish

// Tool publishes an existing remote as a CLI command and MCP tool. Put it in the same
// *.remote.go file as the skgo.Query or skgo.Command declaration:
//
//	var _ = skgo.Query(listTasks)
//	var _ = clipublish.Tool(listTasks)
//
// Generation reads the function's resolved identity, signature and Go comments.
// Tool names stay unchanged; duplicate names produce conflicting generated Go.
func Tool(fn any) struct{} { _ = fn; return struct{}{} }
