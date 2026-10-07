package app

import (
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

type Locals struct {
	Session businesslogic.Session
	Visit   businesslogic.Visit
	Visitor string
}

type RequestEvent[P any] = skgo.RequestEvent[P, Locals]
