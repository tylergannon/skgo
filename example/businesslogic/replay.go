package businesslogic

import "sync"

// ReplayLog counts, per run, how many times the app answered each request that
// named the run in its query. It is what lets a scenario state how many times
// Go was reached for a universal fetch — the cold document, the browser's
// hydration and a client navigation each either make a request or reuse the
// one the document carried — without asking the page that made them.
type ReplayLog struct {
	mu     sync.Mutex
	counts map[string]map[string]int
}

// Replays is the app's log.
var Replays = &ReplayLog{counts: map[string]map[string]int{}}

// Record notes one request, keyed "METHOD /path", under run.
func (l *ReplayLog) Record(run, request string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[run] == nil {
		l.counts[run] = map[string]int{}
	}
	l.counts[run][request]++
}

// Counts is a copy of what run has recorded.
func (l *ReplayLog) Counts(run string) map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]int{}
	for request, n := range l.counts[run] {
		out[request] = n
	}
	return out
}
