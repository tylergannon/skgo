package hooks

import (
	"fmt"
	"strconv"

	"github.com/tylergannon/skgo/example/domain"
	"github.com/tylergannon/skgo/example/legacy"
	other "github.com/tylergannon/skgo/example/other"
)

func Numeric(s string) (domain.Number, bool) {
	n, e := strconv.Atoi(s)
	return domain.Number(n), e == nil
}
func Aliased(s string) (domain.NumberAlias, bool) { return Numeric(s) }
func Other(s string) (other.Number, bool)         { return 42, true }
func Flag(s string) (bool, bool)                  { return false, true }
func Blank(s string) (string, bool)               { return "", true }
func MaybeRef(s string) (domain.Ref, bool) {
	if s == "none" {
		return nil, true
	}
	if s == "42" {
		return &domain.OrderRef{Number: 42}, true
	}
	return nil, false
}
func Pointer(s string) (*domain.OrderRef, bool) { return nil, true }
func Generic(s string) (domain.Box[domain.Number], bool) {
	return domain.Box[domain.Number]{Value: 42}, true
}
func Structure(s string) (struct {
	Value int `json:"value"`
}, bool) {
	return struct {
		Value int `json:"value"`
	}{42}, true
}
func Function(s string) (func(number int) string, bool) {
	return func(n int) string { return fmt.Sprint(n) }, true
}
func FunctionAlias(s string) (func(value int) string, bool)  { return Function(s) }
func Slice(s string) ([]domain.Number, bool)                 { return nil, true }
func Map(s string) (map[string]domain.Number, bool)          { return nil, true }
func Channel(s string) (<-chan domain.Number, bool)          { return nil, true }
func Interface(s string) (interface{ Label() string }, bool) { return nil, true }
func Array(s string) ([2]domain.Number, bool)                { return [2]domain.Number{0, 42}, true }
func StructOrder(s string) (domain.Order, bool)              { return domain.Order{Number: 42}, true }
func LegacyOrder(s string) (legacy.Order, bool)              { return legacy.Order{Number: 17}, true }
func PublicHidden(s string) (domain.PublicHidden, bool)      { return 0, true }
