package dev

import (
	"encoding/base32"
	"path/filepath"
	"regexp"
	"strings"
)

// A route directory Go cannot spell is compiled from a copy under the
// generated bindings' `links/` tree, named by lowercase base32 of the
// vite-root-relative path (internal/gen/links.go). The compiler therefore
// reports an error in the copy, which a developer never opened.
//
// The name is injective and decodable, so the report can be turned back into
// the file that was actually written without consulting anything that might be
// stale after a failed generation.
var linkPath = regexp.MustCompile(`(?:[\w.\-/]*/)?links/([a-z2-7]+)(/?)`)

// authoredPaths rewrites every link-tree path in a compiler or generator
// report to the authored file beside the route it stands for. Paths are shown
// relative to the module root, where the developer runs the tools.
func authoredPaths(report, root, web string) string {
	return linkPath.ReplaceAllStringFunc(report, func(match string) string {
		groups := linkPath.FindStringSubmatch(match)
		decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(groups[1]))
		if err != nil || !strings.HasPrefix(string(decoded), "src") {
			return match
		}
		dir := filepath.Join(web, string(decoded))
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			rel = dir
		}
		return filepath.ToSlash(rel) + groups[2]
	})
}
