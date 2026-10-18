package domain

import "fmt"

type Number int

func (n Number) Label() string { return fmt.Sprintf("Order #%d", n) }

type NumberAlias = Number
type Order struct{ Number int }

func (r Order) Label() string { return fmt.Sprintf("Struct order #%d", r.Number) }

type OrderRef struct{ Number int }

func (r *OrderRef) Label() string { return fmt.Sprintf("Ref #%d", r.Number) }

type Ref interface{ Label() string }
type Box[T any] struct{ Value T }
type hidden int
type PublicHidden = hidden
