package render

import (
	"regexp"
	"strings"
)

// slugify is svelte.dev's heading slug, which the site also applies: case is
// kept, "<", ">" and "..." are dropped, every other character outside
// [A-Za-z0-9-$(.):'_] becomes "-", runs collapse and the ends are trimmed.
func slugify(text string) string {
	text = strings.ReplaceAll(text, "’", "'")
	text = strings.Join(strings.Fields(text), " ")
	text = strings.NewReplacer("<", "", ">", "", "...", "").Replace(text)
	text = notSlug.ReplaceAllString(text, "-")
	text = dashes.ReplaceAllString(text, "-")
	return strings.Trim(text, "-")
}

var (
	notSlug = regexp.MustCompile(`[^A-Za-z0-9\-$(.):'_]`)
	dashes  = regexp.MustCompile(`-{2,}`)
	heading = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
)

// headingIDs lists the anchors of a rendered markdown body the way the site
// nests them: a heading's slug joined to the slugs of the open headings above
// it, so an h3 under an h2 is "<h2>-<h3>".
func headingIDs(markdown string) map[string]bool {
	ids := map[string]bool{}
	var stack []string
	inFence := false
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := heading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		level := len(m[1])
		for len(stack) < level {
			stack = append(stack, "")
		}
		stack = stack[:level]
		stack[level-1] = slugify(plainText(m[2]))
		var parts []string
		for _, s := range stack {
			if s != "" {
				parts = append(parts, s)
			}
		}
		ids[strings.Join(parts, "-")] = true
	}
	return ids
}

var inlineMarkup = regexp.MustCompile("`([^`]*)`|\\[([^\\]]*)\\]\\([^)]*\\)|[*_]{1,2}([^*_]+)[*_]{1,2}")

// plainText strips inline code, links and emphasis from heading text.
func plainText(s string) string {
	return inlineMarkup.ReplaceAllStringFunc(s, func(m string) string {
		sub := inlineMarkup.FindStringSubmatch(m)
		for _, g := range sub[1:] {
			if g != "" {
				return g
			}
		}
		return ""
	})
}
