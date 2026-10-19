// Package form is the form sample behind the remote functions guide: a
// message form with an attachment, validated in Go.
package form

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic/visitor"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

// Draft is what the form submits. Each field's json tag is the path the page
// uses in `sendMessage.fields...`, which is what joins a control to a Go field.
type Draft struct {
	// From is the `from` text input.
	From string `json:"from"`
	// Email is the `email` input.
	Email string `json:"email"`
	// Body is the `body` textarea.
	Body string `json:"body"`
	// Attachment is the `attachment` file input. Its bytes are already read
	// by the time sendMessage runs.
	Attachment skgo.File `json:"attachment"`
	// ForKey is the key of a `sendMessage.for(key)` instance, which Kit
	// delivers as the argument's `id` field. It is empty for the bare form.
	ForKey string `json:"id"`
}

// Message is one message that was sent.
type Message struct {
	// From is the sender's name.
	From string `json:"from"`
	// Body is what they wrote.
	Body string `json:"body"`
	// Attachment is the uploaded file's name, or empty when there was none.
	Attachment string `json:"attachment"`
}

// inbox holds the messages each visitor has sent, keyed by visitor id.
var inbox = struct {
	sync.Mutex
	byVisitor map[string][]Message
}{byVisitor: map[string][]Message{}}

// getMessages lists the messages this visitor has sent so far.
func getMessages(ctx context.Context) ([]Message, error) {
	inbox.Lock()
	defer inbox.Unlock()
	return slices.Clone(inbox.byVisitor[visitor.Of(ctx)]), nil
}

// sendMessage records a message. Each issue added to invalid is shown on the
// field it names, and the form keeps what the visitor typed.
func sendMessage(ctx context.Context, event params.RequestEvent, draft Draft) (Message, error) {
	invalid := &skgo.Invalid{}
	if strings.TrimSpace(draft.From) == "" {
		invalid.Add("from", "Tell us who you are")
	}
	if !strings.Contains(draft.Email, "@") {
		invalid.Add("email", "%q is not an email address", draft.Email)
	}
	if strings.TrimSpace(draft.Body) == "" {
		invalid.Add("body", "Write a message")
	}
	if err := invalid.Err(); err != nil {
		return Message{}, err
	}

	message := Message{From: draft.From, Body: draft.Body, Attachment: draft.Attachment.Name}
	inbox.Lock()
	who := visitor.Of(ctx)
	inbox.byVisitor[who] = append(inbox.byVisitor[who], message)
	inbox.Unlock()
	return message, skgo.RefreshRequestedNoArg(ctx, getMessages)
}

var (
	_ = skgo.Query(getMessages)
	_ = skgo.Form(sendMessage)
)
