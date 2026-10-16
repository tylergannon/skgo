// Applications select an optional tooling main with skgo dev --generator-config
// dev-generator.json. The configuration is application-owned JSON:
//
//	{
//	  "package": "./internal/tools/devgen",
//	  "beforeBuild": {"dir": ".", "argv": ["go", "run", "./internal/tools/bootstrap"]}
//	}
//
// Package and command directories resolve from the application root. beforeBuild
// is optional and runs directly as argv, without a shell, before helper builds;
// "go" uses skgo dev's --go executable. Use a targeted prerequisite when stale
// generated code would otherwise prevent the helper itself from compiling.
//
// The tooling main calls Serve with its retained callback. SKGO owns watching,
// serialized selection, helper refresh, and application lifecycle. An observed
// edit during generation stays pending for another pass. True requires a full
// application replacement; errors retain the healthy application and require a
// full replacement after repair, even if that later callback returns false.
// Helpers must complete a response within five minutes. Cancellation, invalid
// stdout, or helper exit fails the pass and closes its process group.
package devgen
