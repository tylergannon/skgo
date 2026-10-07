package gen

import (
	"fmt"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// endpointMarkers are the marker names that declare one method of a server
// route, and the method each one declares. They are kit's own export names —
// its ENDPOINT_METHODS plus `fallback` — because those names *are* the
// interface: kit dispatches by looking the request's method up on the compiled
// module's exports.
var endpointMarkers = map[string]string{
	"GET":      "GET",
	"POST":     "POST",
	"PUT":      "PUT",
	"PATCH":    "PATCH",
	"DELETE":   "DELETE",
	"OPTIONS":  "OPTIONS",
	"HEAD":     "HEAD",
	"QUERY":    "QUERY",
	"Fallback": "fallback",
}

// endpointMethodOrder is kit's ENDPOINT_METHODS order, with the fallback last.
// The generated module's exports are written in it so that a regenerated file
// does not reorder itself.
var endpointMethodOrder = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD", "QUERY", "fallback"}

// routesDir is the vite-root-relative directory kit reads routes from. Kit's
// route id is the route directory's path below it, verbatim — groups and
// parameters included — because kit walks the tree and joins the id straight
// back onto the directory (`core/sync/create_manifest_data/index.js`).
const routesDir = "src/routes"

// serverFileName is the Go file a server route is written in. It is
// `+server.ts` minus the `+`, which Go refuses in a file name, exactly as
// page.server.go and layout.server.go are named for the files they generate.
// The app's language decides whether it generates `+server.ts` or `+server.js`.
const serverFileName = "server.go"

// endpointFn is one declared method of one server route.
type endpointFn struct {
	// method is the kit export name: an HTTP method, or "fallback".
	method string
	// name is the Go identifier of the handler.
	name  string
	goPkg *goPackage
	// module is the vite-root-relative path of the `+server.ts`/`+server.js`
	// that will carry it, e.g. "src/routes/haiku/+server.ts".
	module string
	// stub is the absolute path of that file.
	stub string
	// routeID is kit's id for the route, e.g. "/haiku" or "/(marketing)/plans".
	routeID string
	pos     token.Position
}

// routeIDFor turns the vite-root-relative path of a `+server.ts` — or its
// `+server.js` counterpart in a JavaScript app — into kit's route id.
//
// Kit's own walk builds the id by joining directory names below `src/routes`
// and then reads that directory back with `path.join(cwd, routes_base, id)`
// (`core/sync/create_manifest_data/index.js`), so the id is the directory path
// verbatim: group segments like `(marketing)` and parameter segments like
// `[id]` are part of it, and the routes root itself is `/`.
func routeIDFor(module string) (string, error) {
	if !strings.HasPrefix(module, routesDir+"/") {
		return "", fmt.Errorf("skgo: a server route must live under %s, but %s does not", routesDir, module)
	}
	dir := strings.TrimPrefix(module, routesDir)
	dir = strings.TrimSuffix(dir, "/+server.ts")
	dir = strings.TrimSuffix(dir, "/+server.js")
	if dir == "" {
		return "/", nil
	}
	return dir, nil
}

// writeEndpointStubs emits one `+server.ts` per `server.go` — or one
// `+server.js` whose declarations carry JSDoc types when the app is JavaScript.
//
// The exports have to be named as kit names them and they have to survive being
// imported: kit's build reads the compiled module's exports to learn which
// methods the route answers (`core/postbuild/analyse.js`, `analyse_endpoint`),
// and its server looks the request's method up on the same object
// (`runtime/server/endpoint.js`). It never calls them. So the bodies throw, and
// a real response is proof the Go handler answered.
func (a *app) writeEndpointStubs() error {
	byStub := map[string][]*endpointFn{}
	for _, ep := range a.endpoints {
		byStub[ep.stub] = append(byStub[ep.stub], ep)
	}
	var stubs []string
	for stub := range byStub {
		stubs = append(stubs, stub)
	}
	sort.Strings(stubs)

	for _, stub := range stubs {
		var b strings.Builder
		b.WriteString(tsHeader)
		b.WriteString("import { building } from '$app/env';\n")
		b.WriteString("// Runtime bodies throw. Kit build callbacks forward to Go.\n")
		b.WriteString("// Every response is produced by the Go handler rather than\n")
		b.WriteString("// this module. Kit reads the export names to learn which methods the route\n")
		b.WriteString("// answers.\n")
		if a.cfg.Language.JavaScript() {
			// A JavaScript module has no return-type syntax, so the body's
			// `never` is declared in JSDoc. Kit's `RequestHandler` is the same
			// type a `.ts` stub is checked against, so every export — including
			// `QUERY` and `fallback` — type-checks as one.
			b.WriteString("/**\n * @returns {never}\n */\nconst unimplemented = () => {\n\tthrow new Error('skgo: implemented in Go');\n};\n")
		} else {
			b.WriteString("const unimplemented = (): never => {\n\tthrow new Error('skgo: implemented in Go');\n};\n")
		}

		if a.cfg.Language.JavaScript() {
			b.WriteString("/** @param {import('@sveltejs/kit').RequestEvent} event */\nconst fromGo = (event) => {\n const platform = /** @type {any} */ (event.platform);\n return building ? platform?.skgoPrerenderEndpoint?.(event) ?? unimplemented() : unimplemented();\n};\n")
		} else {
			b.WriteString("const fromGo = (event: import('@sveltejs/kit').RequestEvent): Promise<Response> => {\n const platform = event.platform as { skgoPrerenderEndpoint?: (event: import('@sveltejs/kit').RequestEvent) => Promise<Response> } | undefined;\n return building ? platform?.skgoPrerenderEndpoint?.(event) ?? unimplemented() : unimplemented();\n};\n")
		}

		for _, method := range endpointMethodOrder {
			for _, ep := range byStub[stub] {
				if ep.method != method {
					continue
				}
				if a.cfg.Language.JavaScript() {
					fmt.Fprintf(&b, "\n/** @type {import('@sveltejs/kit').RequestHandler} */\nexport const %s = fromGo;\n", method)
				} else {
					fmt.Fprintf(&b, "\nexport const %s = fromGo;\n", method)
				}
			}
		}
		if err := a.write(stub, b.String()); err != nil {
			return err
		}
		if err := a.removeObsoleteStub(stub); err != nil {
			return err
		}
	}
	return nil
}

// checkEndpointDuplicates refuses two handlers for the same method of the same
// route. Kit could not express it either: a module has one export per name.
func (a *app) checkEndpointDuplicates() error {
	seen := map[string]*endpointFn{}
	for _, ep := range a.endpoints {
		key := ep.module + "#" + ep.method
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("skgo: %s answers %s twice, at %s and %s", ep.module, ep.method, prev.pos, ep.pos)
		}
		seen[key] = ep
	}
	return nil
}

// endpointList is what the adapter compares against kit's own build: per route
// id, the methods Go answers, spelled the way kit's build spells them — its
// method names, and `"*"` for a fallback.
func (a *app) endpointList() map[string][]string {
	out := map[string][]string{}
	for _, ep := range a.endpoints {
		method := ep.method
		if method == "fallback" {
			method = "*"
		}
		out[ep.routeID] = append(out[ep.routeID], method)
	}
	for _, methods := range out {
		sort.Strings(methods)
	}
	return out
}

// endpointsByPackage groups the declared methods by the Go package that
// declares them, for the per-package registration file.
func (a *app) endpointsByPackage() map[*goPackage][]*endpointFn {
	byPkg := map[*goPackage][]*endpointFn{}
	for _, ep := range a.endpoints {
		byPkg[ep.goPkg] = append(byPkg[ep.goPkg], ep)
	}
	return byPkg
}

// endpointStubPath is the `+server.ts` — or `+server.js` in a JavaScript app —
// that a `server.go` generates. The app's language decides the extension, the
// same way it decides a remote or load stub's.
func endpointStubPath(path string, lang Language) string {
	return filepath.Join(filepath.Dir(path), "+server"+lang.ext())
}
