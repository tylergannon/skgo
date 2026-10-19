// Package lang is a route whose [[lang]] segment is optional: /lang and
// /lang/fr both reach the same load.
package lang

import "github.com/tylergannon/skgo"

type PageData struct {
	Lang string `json:"lang"`
}

func pageLoad(event PageRequestEvent) (PageData, error) {
	lang := "en"
	if value := event.Params.Lang(); value != nil {
		lang = *value
	}
	return PageData{Lang: lang}, nil
}

var _ = skgo.Load(pageLoad)
