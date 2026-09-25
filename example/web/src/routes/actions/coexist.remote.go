package actions

import (
	"context"
	"strings"

	"github.com/tylergannon/skgo"
)

type RemoteNote struct {
	Name string `json:"name"`
}

type RemoteReceipt struct {
	Message string `json:"message"`
}

func sendRemoteNote(_ context.Context, note RemoteNote) (RemoteReceipt, error) {
	return RemoteReceipt{Message: "Remote Go form received " + strings.TrimSpace(note.Name)}, nil
}

var _ = skgo.Form(sendRemoteNote)
