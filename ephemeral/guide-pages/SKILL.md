# Writing guide pages

Guide pages are markdown files committed beside the code they describe: core
pages in the module root, CLI and AI pages in `cmd/skgo/`, a page about
another package in that package's directory. Location carries no meaning; the
frontmatter does. Keys: `title`, `description` (one sentence, under twenty
words) and `division` (`skgo`, `cli` or `ai`) are required; `section`,
`subsection`, `slug` (when it differs from the file name) and `order` place the
page in the site's navigation. Reference documentation is godoc.

Check every claim against current code, tests or the pinned Kit source, never
against another document. Write plain claims: no slogans or pitch lines, and
no status narration ("now", "no longer", "recently", "as of"). Setup detail
meant for a user's agent belongs in `skills/skgo/SKILL.md`, not the README.

A page never contains Go, Svelte or TypeScript text. A `go`, `svelte` or `ts`
fence has an empty body and an info string naming its source: a Go Example
function as go/doc names it (`ExampleLoad_sharedState`, `generate.ExamplePrepare`),
or a whole file by its path from the repository root
(`example/web/src/routes/invoices/+page.svelte`). Examples live in `_test.go`
files in the external test package, so `go test` compiles them and checks any
`// Output:`. A `text` fence whose info string is an `skgo` command line (`skgo
new -h`) also has an empty body: the site runs that command with the release's
binary in an empty directory and inserts what it prints, so a page never pastes
command output. The project site (tylergannon/skgo-project) extracts pages,
samples and godoc from the tagged release when it builds; nothing derived is
committed here.
