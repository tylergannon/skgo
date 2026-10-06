package hooks

import (
	"fmt"
	"strconv"
)

// OrderNumber keeps its identity and methods in a generated load event.
type OrderNumber int64

func (n OrderNumber) Label() string { return fmt.Sprintf("Order #%d", n) }

// Order is paired with the declared Order matcher in params.ts.
func Order(value string) (OrderNumber, bool) {
	n, err := strconv.ParseInt(value, 10, 64)
	return OrderNumber(n), err == nil && n >= 0 && n <= 1000000
}
