package render

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tylergannon/skgo/internal/commands"
)

var (
	packageLine  = regexp.MustCompile(`^> PACKAGE:\s*(\S+)\s*$`)
	symbolsLine  = regexp.MustCompile(`^> SYMBOLS:\s*(.*?)\s*$`)
	commandLine  = regexp.MustCompile(`^> COMMAND:\s*skgo\s+(.+?)\s*$`)
	commandsLine = regexp.MustCompile(`^> COMMANDS:\s*$`)
)

// reference is one `> PACKAGE:` placeholder of a page.
type reference struct {
	Line       int // index into the page body lines
	ImportPath string
	Symbols    []string // nil for the whole package
	span       int      // lines the placeholder occupies (1 or 2)
}

// references finds a page's reference placeholders. The body is scanned
// outside fences.
func references(page *Page) ([]reference, []Problem) {
	var refs []reference
	var problems []Problem
	lines := strings.Split(page.Body, "\n")
	inFence := false
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if symbolsLine.MatchString(lines[i]) {
			problems = append(problems, Problem{File: page.File, Line: page.bodyLine + i, Message: "SYMBOLS: must follow a PACKAGE: line"})
			continue
		}
		m := packageLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		ref := reference{Line: i, ImportPath: m[1], span: 1}
		if i+1 < len(lines) {
			if sm := symbolsLine.FindStringSubmatch(lines[i+1]); sm != nil {
				ref.Symbols = strings.Fields(sm[1])
				if len(ref.Symbols) == 0 {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + i + 1, Message: "SYMBOLS: lists at least one symbol; omit the line for the whole package"})
				}
				ref.span = 2
				i++
			}
		}
		refs = append(refs, ref)
	}
	return refs, problems
}

// commandTable renders `> COMMAND: skgo NAME`.
func commandTable(name string) (string, error) {
	c, ok := commands.Lookup(name)
	if !ok || c.Internal {
		return "", fmt.Errorf("no command %q in internal/commands", name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "```sh\n%s\n```\n\n%s\n", c.Synopsis(), strings.TrimSpace(c.Description))
	if len(c.Flags) > 0 {
		b.WriteString("\n| Flag | Default | Description |\n|---|---|---|\n")
		for _, f := range c.Flags {
			def := ""
			if f.Default != "" {
				def = "`" + f.Default + "`"
			}
			fmt.Fprintf(&b, "| `--%s` | %s | %s |\n", f.Name, def, strings.ReplaceAll(f.Usage, "|", "\\|"))
		}
	}
	return b.String(), nil
}

// commandsTable renders `> COMMANDS:`, linking each command to its page in
// the cli division.
func commandsTable() string {
	var b strings.Builder
	b.WriteString("| Command | Does |\n|---|---|\n")
	for _, c := range commands.All() {
		fmt.Fprintf(&b, "| [`%s`](%s) | %s |\n", "skgo "+c.Name, commandSlug(c.Name), c.Summary)
	}
	return b.String()
}

func commandSlug(name string) string { return strings.ReplaceAll(name, " ", "-") }

// expandCommands replaces the command directives in a body.
func expandCommands(page *Page, body string) (string, []Problem) {
	var problems []Problem
	lines := strings.Split(body, "\n")
	var out []string
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence {
			if m := commandLine.FindStringSubmatch(line); m != nil {
				text, err := commandTable(m[1])
				if err != nil {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + i, Message: err.Error()})
					out = append(out, line)
					continue
				}
				out = append(out, strings.TrimRight(text, "\n"))
				continue
			}
			if commandsLine.MatchString(line) {
				out = append(out, strings.TrimRight(commandsTable(), "\n"))
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), problems
}
