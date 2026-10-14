package gen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

type projectionFile struct {
	source     [32]byte
	projection [32]byte
}

// projectionKey is deliberately broader than type reachability. Initializer
// values, imports, comments, tags, excluded files, membership, module controls
// and non-Go inputs remain exact. Only named function bodies are omitted: SKGO
// inspects their signatures and declarations, never their implementation.
// This is not a compiler-validity cache. The dev owner must compile the fresh
// executable sources after Run, including after a projection hit.
func projectionKey(input, environment string, metadata *kitRouteMetadata, files map[string][]byte) (string, error) {
	return projectionKeyMemo(input, environment, metadata, files, nil, nil, nil)
}

func (w *Worker) projectionKey(input, environment string, metadata *kitRouteMetadata, files map[string][]byte) (string, error) {
	next := make(map[string]projectionFile)
	key, err := projectionKeyMemo(input, environment, metadata, files, w.projectionFiles, next, &w.stats.ProjectionParses)
	// Retain only this capture, including on failure; repeated deleted/broken
	// inputs must not accumulate historical file or compiler state.
	w.projectionFiles = next
	return key, err
}

func projectionKeyMemo(input, environment string, metadata *kitRouteMetadata, files map[string][]byte, previous, next map[string]projectionFile, parses *uint64) (string, error) {
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode([]string{input, environment})
	_ = enc.Encode(metadata)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		data := files[path]
		_ = enc.Encode(path)
		if data == nil {
			_ = enc.Encode(nil)
			continue
		}
		sourceHash := sha256.Sum256(data)
		if strings.HasSuffix(path, ".go") {
			if cached, ok := previous[path]; ok && cached.source == sourceHash {
				if next != nil {
					next[path] = cached
				}
				_ = enc.Encode(cached.projection)
				continue
			}
			if parses != nil {
				*parses++
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, data, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return "", err
			}
			var projected []byte
			start := 0
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				begin, end := fset.Position(fn.Body.Pos()).Offset, fset.Position(fn.Body.End()).Offset
				projected = append(projected, data[start:begin]...)
				projected = append(projected, '{', '}')
				start = end
			}
			data = append(projected, data[start:]...)
		}
		sum := sha256.Sum256(data)
		if next != nil && strings.HasSuffix(path, ".go") {
			next[path] = projectionFile{source: sourceHash, projection: sum}
		}
		_ = enc.Encode(sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
