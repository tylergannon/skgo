// Package contact serves the /contact route: a form written in Go.
//
// A form is the one remote kind whose argument the browser builds rather than
// the application. Kit's client reads the `<form>`'s controls, coerces each one
// according to the name `fields.<name>.as(...)` gave it, and posts the result
// as its own binary envelope with any uploaded file's bytes appended raw. Go
// answers that envelope, so the handler below receives a typed Draft with the
// file's contents already in it.
package contact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/tylergannon/skgo"
)

// Message is one message the visitor has sent, as the page lists it.
type Message struct {
	// ID is the message's number in the order it arrived.
	ID string `json:"id"`
	// From is the sender's name.
	From string `json:"from"`
	// Email is the address they gave.
	Email string `json:"email"`
	// Body is what they wrote.
	Body string `json:"body"`
	// Attachment is the uploaded file's name, or empty when there was none.
	Attachment string `json:"attachment"`
	// AttachmentBytes is how many bytes of it reached Go.
	AttachmentBytes int `json:"attachmentBytes"`
	// AttachmentDigest is the first twelve hex digits of the SHA-256 of those
	// bytes. It is on the page because a byte count alone cannot tell an
	// upload that arrived from one that arrived scrambled — the digest changes
	// if a single byte moved, which is exactly the failure kit's
	// smallest-file-first body ordering invites.
	AttachmentDigest string `json:"attachmentDigest"`
}

// Draft is what the form submits. Each field's name matches the path the page
// uses in `sendMessage.fields...`, which is what joins a control to a Go field.
type Draft struct {
	// From is the `from` text input.
	From string `json:"from"`
	// Email is the `email` input.
	Email string `json:"email"`
	// Body is the `body` textarea.
	Body string `json:"body"`
	// Attachment is the `attachment` file input. A File field is how a form
	// receives an upload; the bytes are already in it by the time this
	// function runs.
	Attachment skgo.File `json:"attachment"`
}

// Receipt is what a successful submission returns, which kit's client puts on
// `sendMessage.result`.
type Receipt struct {
	// ID is the new message's id.
	ID string `json:"id"`
	// Summary is a sentence the page can show without re-reading the list.
	Summary string `json:"summary"`
}

var inbox = struct {
	sync.Mutex
	messages []Message
}{}

// getMessages lists everything sent so far, newest last.
func getMessages(_ context.Context, _ skgo.None) ([]Message, error) {
	inbox.Lock()
	defer inbox.Unlock()
	// A copy: the caller serialises this after the lock is gone.
	out := make([]Message, len(inbox.messages))
	copy(out, inbox.messages)
	return out, nil
}

// sendMessage records a message.
//
// The checks below are the form's validation. Returning a *skgo.Invalid puts
// each message on the field it names, and kit's client leaves the page — and
// therefore everything the visitor typed — exactly as it was.
func sendMessage(_ context.Context, draft Draft) (Receipt, error) {
	invalid := &skgo.Invalid{}

	if strings.TrimSpace(draft.From) == "" {
		invalid.Add("from", "Tell us who you are")
	}
	if !strings.Contains(draft.Email, "@") {
		invalid.Add("email", "%q is not an email address", draft.Email)
	}
	if n := len([]rune(strings.TrimSpace(draft.Body))); n < 10 {
		invalid.Add("body", "A message needs at least 10 characters; this one has %d", n)
	}
	if err := invalid.Err(); err != nil {
		return Receipt{}, err
	}

	message := Message{
		From:  strings.TrimSpace(draft.From),
		Email: draft.Email,
		Body:  strings.TrimSpace(draft.Body),
	}
	if draft.Attachment.Name != "" {
		sum := sha256.Sum256(draft.Attachment.Data)
		message.Attachment = draft.Attachment.Name
		message.AttachmentBytes = len(draft.Attachment.Data)
		message.AttachmentDigest = hex.EncodeToString(sum[:])[:12]
	}

	inbox.Lock()
	message.ID = fmt.Sprintf("m%d", len(inbox.messages)+1)
	inbox.messages = append(inbox.messages, message)
	inbox.Unlock()

	return Receipt{ID: message.ID, Summary: "Thanks, " + message.From + " — message " + message.ID + " is in."}, nil
}

var (
	_ = skgo.Query(getMessages)
	_ = skgo.Form(sendMessage)
)
