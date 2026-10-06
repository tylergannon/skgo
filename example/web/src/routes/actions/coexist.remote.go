package actions

import (
	"strings"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

type RemoteNote struct {
	Name string `json:"name"`
}

type RemoteReceipt struct {
	Message string `json:"message"`
}

func sendRemoteNote(_ params.RequestEvent, note RemoteNote) (RemoteReceipt, error) {
	return RemoteReceipt{Message: "Remote Go form received " + strings.TrimSpace(note.Name)}, nil
}

var _ = skgo.Form(sendRemoteNote)
