package templateapi

import (
	"context"
	"go/token"

	"github.com/tylergannon/polytype/typegrammar"
)

// Generator is an optional capability implemented by a Plugin. It receives
// resolved application metadata and proposes files; it must not write files,
// load application packages again, or change server behavior. Honor ctx.
type Generator interface {
	Generate(context.Context, GenerationRequest) (GenerationResult, error)
}

// MarkedGenerator selects its remote surface with top-level marker declarations
// beside the ordinary skgo remote declarations. RemoteMarker returns the resolved
// Go identity "import/path#Function". The marker accepts one local function.
// Core supplies only marked remotes, before projecting their wire types. An
// empty Selections list therefore selects marked remotes rather than all remotes.
// The generator still decides which remote kinds and contracts it supports.
type MarkedGenerator interface {
	Generator
	RemoteMarker() string
}

// GeneratorInvocation selects one installed generator for an invocation.
// ID and Version identify the provider. Selections are remote module paths
// followed by #export; empty selects every eligible remote. MarkedGenerator
// narrows eligibility to its marker. Options belong to the plugin.
type GeneratorInvocation struct {
	ID, Version string
	Generator   Generator
	Selections  []string
	Options     map[string]string
}

// GenerationRequest is an immutable view of one application's selected remotes.
// Plugins must not mutate Model, Selections, Options, or their reachable values.
type GenerationRequest struct {
	// RemoteMarker records the filter actually applied by the host. A marked
	// generator must reject an absent or different value rather than expose
	// an unfiltered model supplied by an unsupported host.
	RemoteMarker string
	Project      Project
	Model        GenerationModel
	Selections   []string
	Options      map[string]string
}

// GenerationModel uses Polytype's public wire grammar. Roots indexes are local
// to this model; definitions contain the reachable named types. Each invocation
// receives a fresh view. This contract adds no route or documentation model.
type GenerationModel struct {
	Definitions typegrammar.Definitions
	Roots       []typegrammar.Type
	Remotes     []RemoteOperation
}

// RemoteOperation describes a resolved remote's identity and wire types.
// ID is Kit's hashed module/export endpoint identity. Kind is query, command,
// query.live, query.batch, form, or prerender. Input and Output index Roots;
// -1 means absent or not representable by this wire grammar (form input or a
// transported value). Transported reports a reachable custom transport.
// WireFormat is "devalue" for ordinary remote calls and "formdata" for forms.
type RemoteOperation struct {
	Selection, Name, Module, ID, Kind, WireFormat string
	// Description is the declared Go function's documentation comment.
	Description   string
	Input, Output int
	Transported   bool
	Source        token.Position
}

// GenerationResult proposes the current complete output for this provider.
// An error diagnostic fails generation, as does a returned error. No files are
// published until all providers succeed and all destinations pass validation.
type GenerationResult struct {
	Files       []GeneratedFile
	Diagnostics []Diagnostic
}

// GeneratedFile is an application-root-relative, slash-separated file path and
// its complete bytes. Absolute paths, traversal, symlink paths, core outputs,
// duplicate destinations, and modifications of authored files are rejected.
// The host tracks ownership and content hashes, removes unchanged stale output,
// and avoids rewriting identical files. Editing generated output makes it
// authored: remove or move it before regenerating. Go files must use another
// filename than core's reserved skgo_gen.go and contain valid complete Go source.
type GeneratedFile struct {
	Path    string
	Content []byte
}

// Diagnostic is a source-located generator message. Severity is "warning" or
// "error"; any other value is rejected. Source may be zero when not applicable.
type Diagnostic struct {
	Severity, Message string
	Source            token.Position
}
