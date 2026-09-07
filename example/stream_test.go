package example_test

import (
	"strings"
	"testing"
)

// The three values src/routes/stream/page.server.go promises, and the strings
// it promises them as. They are written here rather than read off the response
// for the same reason the Gherkin scenarios write them out: an ordering claim
// checked against the order that arrived is no claim at all.
//
// The load numbers them in the order devalue walks its result — devalue sorts
// an object's keys, so digest, then ticker, then the forecast inside the
// weather panel — and makes them settle in the other order.
var (
	promisedInOrderOfArrival = []string{
		"the first thing to arrive",
		"the second thing to arrive",
		"the last thing to arrive",
	}
	// The chunk ids those three carry, in the order they arrive. Ascending
	// would mean a value that was ready waited behind one that was not.
	idsInOrderOfArrival = []string{"2", "1", "3"}
)

// A cold load gets the page at once, with its loading states already rendered,
// and each promised value follows it down on the same response as it settles.
//
// The nested one matters as much as the order: the forecast is not a field of
// what the load returned, it is a field of the weather panel inside it, which
// is where kit lets a promise sit.
func TestTheDocumentCarriesTheLoadingStatesAndTheValuesFollowIt(t *testing.T) {
	h := newProdHandler(t)
	body := get(t, h, "/stream").Body.String()

	end := strings.Index(body, "</html>")
	if end < 0 {
		t.Fatalf("the response carried no document:\n%s", body)
	}
	document, appended := body[:end], body[end:]

	for _, state := range []string{"ticker-pending", "digest-pending", "forecast-pending"} {
		if !strings.Contains(document, `data-testid="`+state+`"`) {
			t.Errorf("the document has no %s", state)
		}
	}
	for _, value := range promisedInOrderOfArrival {
		if strings.Contains(document, value) {
			t.Errorf("%q was in the document, so the page never had a loading state for it", value)
		}
	}

	if got := arrivals(appended, promisedInOrderOfArrival); !equal(got, idsInOrderOfArrival) {
		t.Errorf("the document's chunks arrived as %v, want %v\n%s", got, idsInOrderOfArrival, appended)
	}
}

// The same claim about the response a client-side navigation gets. Kit's client
// asks for __data.json and reads it as it arrives, so the order is the order
// the browser fills the page in — and it has to be the order a cold load fills
// it in, or the same page behaves differently depending on how it was reached.
func TestTheDataResponseCarriesTheValuesInTheOrderTheySettle(t *testing.T) {
	h := newProdHandler(t)
	rec := get(t, h, "/stream/__data.json?x-sveltekit-invalidated=01")

	if ct := rec.Header().Get("Content-Type"); ct != "text/sveltekit-data" {
		t.Errorf("Content-Type = %q, want text/sveltekit-data", ct)
	}
	body := rec.Body.String()
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want a head and three chunks:\n%s", len(lines), body)
	}
	// The head names three promises and carries none of their values.
	if n := strings.Count(lines[0], `["Promise"`); n != 3 {
		t.Errorf("the head names %d promises, want 3:\n%s", n, lines[0])
	}
	for _, value := range promisedInOrderOfArrival {
		if strings.Contains(lines[0], value) {
			t.Errorf("%q was in the head, so it was not promised at all", value)
		}
	}

	if got := arrivals(strings.Join(lines[1:], "\n"), promisedInOrderOfArrival); !equal(got, idsInOrderOfArrival) {
		t.Errorf("the data response's chunks arrived as %v, want %v\n%s", got, idsInOrderOfArrival, body)
	}
}

// arrivals reads the chunk id each of values arrived under, in the order the
// values appear in text. It works on both wires because both spell the id
// beside the value: a document appends `resolve(<id>, () => [<value>])` and a
// data response writes `{"type":"chunk","id":<id>,"data":<value>}`.
func arrivals(text string, values []string) []string {
	type arrival struct {
		at int
		id string
	}
	var found []arrival
	for _, line := range strings.Split(text, "\n") {
		var carries bool
		for _, value := range values {
			if strings.Contains(line, value) {
				carries = true
			}
		}
		if !carries {
			continue
		}
		found = append(found, arrival{at: len(found), id: idOf(line)})
	}
	ids := make([]string, len(found))
	for i, a := range found {
		ids[i] = a.id
	}
	return ids
}

// idOf is the number a chunk names itself by, on either wire.
func idOf(line string) string {
	for _, prefix := range []string{`.resolve(`, `"id":`} {
		i := strings.Index(line, prefix)
		if i < 0 {
			continue
		}
		rest := line[i+len(prefix):]
		end := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
		if end <= 0 {
			continue
		}
		return rest[:end]
	}
	return "<no id in " + line + ">"
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
