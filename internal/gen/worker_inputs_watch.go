package gen

import "slices"

// WorkerInputs describes the latest captured application source selection.
// Roots are mutable module directories, including local replacements. Files
// contains captured source/control paths; a trailing separator denotes directory
// membership. EmbedPatterns are absolute Go embed patterns with all: removed;
// consumers must detect newly matching files as well as changes to current ones.
// Installed versioned dependencies and toolchains are immutable for the session.
type WorkerInputs struct {
	Roots         []string
	Files         []string
	EmbedPatterns []string
}

// Inputs returns independent slices, including after a persistent cache hit.
// It performs no discovery or filesystem reads. Call after Run has returned.
func (w *Worker) Inputs() WorkerInputs {
	w.mu.Lock()
	defer w.mu.Unlock()
	return WorkerInputs{Roots: slices.Clone(w.inputs.Roots), Files: slices.Clone(w.inputs.Files), EmbedPatterns: slices.Clone(w.inputs.EmbedPatterns)}
}
