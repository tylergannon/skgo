package gen

import (
	"reflect"

	"github.com/tylergannon/polytype/typegrammar"
)

// WorkerStats counts source captures, parsing, backend calls and omissions. Loading and lowering
// may still be needed after a declaration edit; a PlanHit omits those too.
type WorkerStats struct {
	PlanHits                       uint64
	SourceCaptures                 uint64
	ProjectionParses               uint64
	TypeScriptRuns, TypeScriptHits uint64
	CodecRuns, CodecHits           uint64
}

func (w *Worker) Stats() WorkerStats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stats
}

type typeProjection struct {
	definitions typegrammar.Definitions
	names       map[typegrammar.Name]string
	files       []projectedFile
	used        uint64
}

type codecProjection struct {
	definitions             typegrammar.Definitions
	roots                   []typegrammar.Type
	packageName, importPath string
	source                  []byte
	used                    uint64
}

func (w *Worker) projectedTypes(key string, defs typegrammar.Definitions) *typeProjection {
	if cached := w.types[key]; cached != nil && reflect.DeepEqual(cached.definitions, defs) {
		cached.used = w.revision
		w.stats.TypeScriptHits++
		return cached
	}
	w.stats.TypeScriptRuns++
	return nil
}

func (w *Worker) projectedCodecs(key, pkg, imp string, defs typegrammar.Definitions, roots []typegrammar.Type) []byte {
	if cached := w.codecs[key]; cached != nil && cached.packageName == pkg && cached.importPath == imp && reflect.DeepEqual(cached.definitions, defs) && reflect.DeepEqual(cached.roots, roots) {
		cached.used = w.revision
		w.stats.CodecHits++
		return cached.source
	}
	w.stats.CodecRuns++
	return nil
}
