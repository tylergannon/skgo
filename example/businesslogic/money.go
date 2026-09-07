package businesslogic

import "strconv"

// Money is an amount of US dollars, held in cents so that arithmetic on it is
// exact.
//
// It is the app's example of a domain type with behaviour: `Format` is a method,
// not a field, so a page that renders a price is calling something rather than
// reading something. That is the whole point of the `transport` hook — without
// one this arrives in the browser as `{ cents: 2000 }`, an object that has no
// `format` to call.
type Money struct {
	// Cents is the amount, in cents.
	Cents int `json:"cents"`
}

// USD is an amount in whole dollars.
func USD(dollars int) Money { return Money{Cents: dollars * 100} }

// Format renders the amount the way a price is written.
func (m Money) Format() string {
	sign := ""
	cents := m.Cents
	if cents < 0 {
		sign, cents = "-", -cents
	}
	whole := strconv.Itoa(cents / 100)
	fraction := strconv.Itoa(cents % 100)
	if len(fraction) < 2 {
		fraction = "0" + fraction
	}
	return sign + "$" + whole + "." + fraction
}
