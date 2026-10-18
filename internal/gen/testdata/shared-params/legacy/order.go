package legacy

import "fmt"

type Order struct{ Number int }

func (r Order) Label() string { return fmt.Sprintf("Legacy order #%d", r.Number) }
