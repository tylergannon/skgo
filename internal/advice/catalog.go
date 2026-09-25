package advice

import "runtime/debug"

// Entry is the repair guidance shipped with this skgo executable. Examples
// show valid skgo APIs; they are illustrative handler fragments, not edits.
type Entry struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	KitVersion  string `json:"kitVersion"`
	Consequence string `json:"consequence"`
	Repair      string `json:"repair"`
	Example     string `json:"example"`
}

const kitVersion = "3.0.0-next.27"

func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// Catalog lists exactly the stable advice rules supported by this binary.
func Catalog() []Entry {
	entries := []Entry{
		{QueryCookie, "Cookie mutation in a query", "", "", "Kit refuses cookie writes in queries; a cached query could also skip a write.", "Move the write into a registered command or form and return its error.", `func signIn(ctx context.Context) (string, error) {
    if err := skgo.EventFrom(ctx).SetCookie("session", "token", skgo.CookieOptions{Path: "/"}); err != nil { return "", err }
    return "signed in", nil
}
var _ = skgo.Command(signIn)`},
		{QueryPageInput, "Page input in a query", "", "", "Kit keys the query cache by its arguments, not the current page URL or route.", "Pass the page value as a typed query argument from the caller.", `func article(ctx context.Context, slug string) (string, error) { return slug, nil }
var _ = skgo.Query(article) // the page calls article(page.params.slug)`},
		{RequestContext, "Lost request context", "", "", "A fresh context has no request event, so event-dependent operations fail or read no request state.", "Use the handler context, or derive a child from it.", `func whoami(ctx context.Context) (string, error) {
    event := skgo.EventFrom(ctx)
    name, _ := event.Cookie("session")
    return name, nil
}
var _ = skgo.Query(whoami)`},
		{OperationError, "Discarded skgo operation error", "", "", "A failed cookie write, refresh, requested update, or live yield is hidden from the caller.", "Return or handle the operation error explicitly.", `func current(ctx context.Context) (string, error) { return "ready", nil }
var _ = skgo.Query(current)
func update(ctx context.Context) (string, error) {
    if err := skgo.RefreshNoArg(ctx, current); err != nil { return "", err }
    return "updated", nil
}
var _ = skgo.Command(update)`},
		{RefreshKind, "Refresh or reconnect target kind", "", "", "skgo rejects a target registered with the wrong remote kind at runtime.", "Refresh a registered Query; reconnect a registered LiveQuery.", `func current(ctx context.Context) (string, error) { return "ready", nil }
var _ = skgo.Query(current)
func watch(ctx context.Context, yield func(string) error) error { return yield("ready") }
var _ = skgo.LiveQuery(watch)
func update(ctx context.Context) (string, error) {
    if err := skgo.RefreshNoArg(ctx, current); err != nil { return "", err }
    if err := skgo.ReconnectRequestedNoArg(ctx, watch); err != nil { return "", err }
    return "updated", nil
}
var _ = skgo.Command(update)`},
		{FormField, "Unknown form issue field", "", "", "Kit attaches an issue only to the matching form field path; a misspelled literal has no matching field.", "Use the form argument's serialized field path, including nested names or array indexes.", `type Signup struct { Email string ` + "`json:\"email\"`" + ` }
func subscribe(ctx context.Context, arg Signup) (string, error) {
    if arg.Email == "" { return "", skgo.Invalidf("email", "required") }
    return "subscribed", nil
}
var _ = skgo.Form(subscribe)`},
		{WireContract, "Unsupported wire value", "", "", "The generator cannot project this value to Kit's client wire contract.", "Use a supported exported value with a unique JSON field name; keep skgo.File in form input and skgo.Deferred in load output only.", `type Result struct { Name string ` + "`json:\"name\"`" + ` }
func read(ctx context.Context) (Result, error) { return Result{Name: "ready"}, nil }
var _ = skgo.Query(read)`},
		{RouteParam, "Unknown route parameter", "", "", "Event.Param returns an empty string for a name this page route does not declare.", "Read a parameter declared in this page's route path.", `// In web/src/routes/[slug]/page.server.go:
func load(ctx context.Context) (string, error) {
    return skgo.EventFrom(ctx).Param("slug"), nil
}
var _ = skgo.Load(load)`},
	}
	for i := range entries {
		entries[i].Version = Version()
		entries[i].KitVersion = kitVersion
	}
	return entries
}

func Lookup(code string) (Entry, bool) {
	for _, entry := range Catalog() {
		if entry.Code == code {
			return entry, true
		}
	}
	return Entry{}, false
}
