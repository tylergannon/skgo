package invoices

import (
	"github.com/tylergannon/skgo"
)

type Invoice struct {
	Number           string `json:"number"`
	DueDate          string `json:"dueDate"`
	OutstandingCents int    `json:"outstandingCents"`
}

type PageData struct {
	AsOf     string    `json:"asOf"`
	Invoices []Invoice `json:"invoices"`
}

func pageLoad(event RequestEvent) (PageData, error) {
	return PageData{
		AsOf: "2026-09-28",
		Invoices: []Invoice{
			{Number: "INV-300", DueDate: "2026-10-15", OutstandingCents: 7400},
			{Number: "INV-100", DueDate: "2026-09-10", OutstandingCents: 12500},
			{Number: "INV-200", DueDate: "2026-09-05", OutstandingCents: 0},
		},
	}, nil
}

var _ = skgo.Load(pageLoad)
