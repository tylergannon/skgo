package app

import "github.com/tylergannon/skgo"

type Locals struct{}

type RequestEvent[P any] = skgo.RequestEvent[P, Locals]
