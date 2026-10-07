package matchers

import (
	"fmt"
	"strconv"
)

type OrderNumber int64

func (n OrderNumber) Label() string { return fmt.Sprintf("Order #%d", n) }
func Order(value string) (OrderNumber, bool) {
	n, err := strconv.ParseInt(value, 10, 64)
	return OrderNumber(n), err == nil && n >= 0 && n <= 1000000
}
