package businesslogic

// VisitCookie is the cookie the example's middleware keeps a visit token in.
const VisitCookie = "skgo_visit"

// FreshVisitToken is what the middleware issues to a visitor whose cookie is
// missing or does not look like a visit token. It is a literal so a test can
// say what it expects without asking the code that produced it.
const FreshVisitToken = "visit-token-7"

// Visit is what the example's middleware establishes for the /middleware
// pages before any load or action runs: the token it authenticated the visit
// with, and the matched route it observed on the request event.
type Visit struct {
	Token string `json:"token"`
	Route string `json:"route"`
	Slug  string `json:"slug"`
	Data  bool   `json:"data"`
}
