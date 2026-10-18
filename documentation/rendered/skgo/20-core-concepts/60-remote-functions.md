---
title: "Remote functions"
summary: "Queries, commands, forms, live and batch queries written in Go, and how they refresh each other."
---

[Remote functions](https://svelte.dev/docs/kit/remote-functions) are functions a component calls as if they were local and that run on the server. In SKGo they are Go functions in a `*.remote.go` file, and `skgo generate` writes the `*.remote.ts` beside it that your components import. The generated bodies throw; the Go function is the only thing that ever answers.

A `var` block at the end of the file says which functions are exposed and as what:

```go
/// file: src/routes/todos/todos.remote.go
var (
	_ = skgo.Query(getTodos)
	_ = skgo.Query(getTodo)
	_ = skgo.Command(addTodo)
	_ = skgo.Command(renameTodo)
	_ = skgo.Command(retitleTodo)
	_ = skgo.LiveQuery(watchCount)
)
```

| Marker              | Kit's kind    | Go signature                                                              |
| ------------------- | ------------- | ------------------------------------------------------------------------- |
| `skgo.Query`        | `query`       | `func(ctx) (T, error)` or `func(ctx, arg A) (T, error)`                   |
| `skgo.BatchQuery`   | `query.batch` | `func(ctx, args []A) ([]T, error)`                                        |
| `skgo.LiveQuery`    | `query.live`  | `func(ctx, yield func(T) error) error`                                    |
| `skgo.Command`      | `command`     | `func(ctx, event params.RequestEvent) (T, error)` or with `arg A` last    |
| `skgo.Form`         | `form`        | `func(ctx, event params.RequestEvent, fields F) (T, error)`               |
| `skgo.Prerender`    | `prerender`   | as `Query`, with `skgo.PrerenderOptions`                                  |

The argument and result types are ordinary Go types, and the generator writes their TypeScript; see [Types and transport](transport).

## query

A `query` reads data. It takes the request's context and, optionally, one argument:

```go
/// file: src/routes/todos/todos.remote.go
// getTodos lists the todos this visitor may see.
func getTodos(ctx context.Context) ([]businesslogic.Todo, error) {
	return visitor.Store(ctx).Todos(signedIn(ctx)), nil
}
```

```go
/// file: src/routes/todos/todos.remote.go
// getTodo looks one todo up by id.
func getTodo(ctx context.Context, id string) (businesslogic.Todo, error) {
	todo, ok := visitor.Store(ctx).Todo(id, signedIn(ctx))
	if !ok {
		return businesslogic.Todo{}, skgo.Errorf(404, "No todo with id %q", id)
	}
	return todo, nil
}
```

```svelte
<!--- file: src/routes/todos/+page.svelte --->
<script lang="ts">
	import { getTodos } from './todos.remote';
</script>

<ul>
	{#each await getTodos() as todo (todo.id)}
		<li>{todo.text}</li>
	{/each}
</ul>
```

Everything Kit says about calling a query holds: `await` it inside a `<svelte:boundary>`, or read `.loading`, `.error` and `.current`; call `.refresh()` to ask for it again; two calls with the same argument in one render are one request. During server rendering, the renderer running inside your Go process calls the Go function directly, so the first value is in the document.

A query may read cookies through `skgo.EventFrom(ctx)` but not write them; a write is refused, and `skgo advice SKGO001` catches it statically. `event.URL()` panics in a query, as `event.url` throws in Kit, because a query is not tied to a page: anything about the page belongs in the argument. See [Using getRequestEvent](#Using-getRequestEvent).

A query that fails returns an error. `skgo.Errorf(status, format, ...)` is an expected error with a status; any other error is a 500 whose message the `handleError` hook decides, as in [Errors](load#Errors).

## query.batch

A `query.batch` answers several calls made in the same turn with one invocation. The Go function receives every argument and returns one result per argument, in the same order:

```go
/// file: src/routes/batch/quotes.remote.go
// getQuotes answers a whole batch of symbols in one call.
//
// The page still asks for one symbol at a time — `getQuotes('SKGO')` — and kit
// collects every such call made in the same turn into this one invocation,
// during a render exactly as in the browser. One result per symbol, in the same
// order: that is how kit's client matches answers to the promises it is
// holding.
func getQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
	out := make([]Quote, len(symbols))
	for i, symbol := range symbols {
		cents, ok := prices[symbol]
		if !ok {
			return nil, skgo.Errorf(404, "No quote for %q", symbol)
		}
		out[i] = Quote{Symbol: symbol, Cents: cents, BatchSize: len(symbols)}
	}
	return out, nil
}

var _ = skgo.BatchQuery(getQuotes)
```

A component still calls `getQuotes('SKGO')` with one symbol; Kit collects the calls, and an error for the batch fails every call in it.

## query.live

A `query.live` pushes values to the client as they change. The Go function receives a `yield` and calls it for every value until it returns or its context is cancelled, which happens when the client disconnects:

```go
/// file: src/routes/todos/todos.remote.go
func watchCount(ctx context.Context, yield func(int) error) error {
	updates, unsubscribe, now := visitor.Store(ctx).Watch(signedIn(ctx))
	defer unsubscribe()

	if err := yield(now.Count); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case next := <-updates:
			if err := yield(next.Count); err != nil {
				return err
			}
		}
	}
}
```

The first value is rendered into the document like any query; the ones after it arrive over the open connection. The event a live query sees is a snapshot of the request that opened it: read the cookie once, at the top, and when it changes have the command that changed it [reconnect the stream](#Single-flight-mutations-Reconnecting-live-queries).

## form

A `form` receives what the browser posted, decoded into a Go struct whose JSON tags match the field paths the page uses:

```go
/// file: src/routes/contact/contact.remote.go
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
	// ForKey is set when the submission came from `sendMessage.for(key)`
	// rather than from the bare `sendMessage` form. No control on the page
	// puts it there: kit delivers a `form.for(key)` instance's key as the
	// `id` field of the argument (`runtime/app/server/remote/form.js`), the
	// same for a submission the browser posted itself as for one kit's
	// client enhanced, which is the whole reason it needs a field here at
	// all — this is Go's side of that json tag, not the page's.
	ForKey string `json:"id"`
}
```

```go
/// file: src/routes/contact/contact.remote.go
// sendMessage records a message.
//
// The checks below are the form's validation. Returning a *skgo.Invalid puts
// each message on the field it names, and kit's client leaves the page — and
// therefore everything the visitor typed — exactly as it was.
func sendMessage(ctx context.Context, event params.RequestEvent, draft Draft) (Receipt, error) {
	caller, err := callerReceipt(event.Params)
	if err != nil {
		return Receipt{}, err
	}
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

	// The page writes `form.submit().updates(getMessages())`; this is the half
	// of that sentence the server gets a say in. A submission that ends in
	// issues never reaches here, and kit's client leaves the page alone.
	return Receipt{
			ID:      message.ID,
			Summary: "Thanks, " + message.From + " — message " + message.ID + " is in.",
			Key:     draft.ForKey,
			Caller:  caller,
			Body:    draft.Body,
		},
		skgo.RefreshRequestedNoArg(ctx, getMessages)
}
```

```svelte
<!--- file: src/routes/contact/ContactForms.svelte --->
<script lang="ts">
	import { getMessages, sendMessage } from './contact.remote';

	const fields = sendMessage.fields;
</script>

<form
	enctype="multipart/form-data"
	{...sendMessage.enhance(async (form) => {
		if (await form.submit().updates(getMessages())) {
			form.element.reset();
		}
	})}
>
	<label>
		Your name
		<input {...fields.from.as('text')} />
	</label>
	{#each fields.from.issues() ?? [] as issue}
		<p class="issue">{issue.message}</p>
	{/each}
	<!-- email, body, attachment -->
	<button>Send</button>
</form>
```

The page side is Kit's: `fields`, `.as(...)`, `issues()`, `enhance`, `for(key)` and multiple submit buttons all work as documented, and a form works with JavaScript off, because the browser posts it and Go answers with the re-rendered document.

A `skgo.File` field receives an uploaded file with its name, type and bytes already read. A keyed instance made with `form.for(key)` delivers its key as the `id` field of the argument, which is what `Draft.ForKey` above is for.

### Validation

The Go struct is the schema: a submission whose values cannot be decoded into it is rejected before the function runs. For rules the type cannot express, return a `*skgo.Invalid`. `invalid.Add(path, format, ...)` attaches a message to a field, `invalid.Err()` returns the error when any were added, and `skgo.Invalidf(path, format, ...)` does both for a single issue. Kit's client puts each message on `fields.<name>.issues()` and leaves the form as the visitor typed it.

### Returns and redirects

What the function returns is `sendMessage.result` on the page. Return a `*skgo.Redirect` to send the visitor elsewhere after a successful submission; Kit's client follows it, and a native submission gets the HTTP redirect.

## command

A `command` writes. It takes the context, the request event and, optionally, one argument, and it may set cookies:

```go
/// file: src/routes/todos/todos.remote.go
// addTodo appends a todo to the list.
//
// The page writes `addTodo(text).updates(getTodos())`, and the line below is
// what makes that request mean anything: a command runs the queries it names
// and no others, however many the browser asks for. There is no limit to
// give: getTodos takes no argument, so kit's client keys it under the empty
// payload and there is exactly one instance of it to ask for.
func addTodo(ctx context.Context, event params.RequestEvent, text string) (businesslogic.Todo, error) {
	if text == "" {
		return businesslogic.Todo{}, skgo.Errorf(400, "A todo needs some text")
	}
	return visitor.Store(ctx).Add(text), skgo.RefreshRequestedNoArg(ctx, getTodos)
}
```

```svelte
<!--- file: src/routes/todos/AddTodo.svelte --->
<script lang="ts">
	import { addTodo, getTodos } from './todos.remote';

	let text = $state('');
</script>

<form
	onsubmit={async (event) => {
		event.preventDefault();
		await addTodo(text).updates(getTodos());
		text = '';
	}}
>
	<input bind:value={text} />
	<button>Add</button>
</form>
```

Kit refuses to run a command while a page renders on the server, so a command is called from an event handler, never from markup. The `event` argument is the generated `params.RequestEvent`, with `Locals` and typed `Params`; see [Using getRequestEvent](#Using-getRequestEvent).

## Single-flight mutations

After a command or form changes something, the queries that showed the old value need new ones. Kit's answer is to return them on the same response, and in SKGo the Go function decides which.

### Server-driven

`skgo.Refresh(ctx, query, arg)` names a query function and the argument of the instance to refresh, in Go, with the query's own parameter type checking the argument:

```go
/// file: src/routes/todos/todos.remote.go
// retitleTodo changes one todo's text and refreshes a getTodo from Go.
//
// The page that calls this passes no `updates(...)`: the browser never says
// what it wants refreshed. The refreshed value on the response is entirely the
// server's doing, and it names the Go function and the argument rather than a
// string — getTodo's own parameter type is what RefreshID has to satisfy.
func retitleTodo(ctx context.Context, event params.RequestEvent, arg Retitle) (businesslogic.Todo, error) {
	todo, ok := visitor.Store(ctx).Rename(arg.ID, arg.Text, signedIn(ctx))
	if !ok {
		return businesslogic.Todo{}, skgo.Errorf(404, "No todo with id %q", arg.ID)
	}
	return todo, skgo.Refresh(ctx, getTodo, arg.RefreshID)
}
```

The page that calls `retitleTodo` passes no `.updates(...)`; the refreshed value is on the response because the server put it there.

### Client-requested

When the page writes `addTodo(text).updates(getTodos())`, Kit's client posts the keys of the query instances it wants back. That list is client input, so the server does not act on it unasked: a command runs the requested queries its handler names, and no others.

- `skgo.RefreshRequestedNoArg(ctx, getTodos)` runs the requested instance of a query that takes no argument
- `skgo.RefreshRequested(ctx, getTodo, 1)` runs up to one requested instance of a query that takes an argument; a request for more comes back refused
- `skgo.IgnoreRequested` and `skgo.IgnoreRequestedNoArg` acknowledge a requested query and deliberately leave it stale, which Kit's client requires for any requested query the command does not run

```go
/// file: src/routes/todos/gate/gate.remote.go
// writeNotes changes both notes and the banner.
//
// The page asks for three instances back — both notes and the banner. One line
// of this handler decides what that request is worth: getNote is named, with a
// limit of one, and each requested getBanner instance is explicitly ignored.
//
// So the left note comes back refreshed; the right note, being the second
// instance of getNote the page asked for, comes back refused with the reason
// on it; and the banner is deliberately not run, which the page shows by going
// stale without making Kit reject the command as unhandled.
func writeNotes(ctx context.Context, event params.RequestEvent, arg Write) (Ack, error) {
	store := store(ctx)
	store.Lock()
	store.notes["left"] = arg.Left
	store.notes["right"] = arg.Right
	store.banner = arg.Banner
	store.Unlock()

	if err := skgo.RefreshRequested(ctx, getNote, 1); err != nil {
		return Ack{}, err
	}
	if err := skgo.IgnoreRequestedNoArg(ctx, getBanner); err != nil {
		return Ack{}, err
	}
	return Ack{Wrote: 3}, nil
}
```

### Reconnecting live queries

A live query's event is a snapshot of the request that opened it, so a command that changes what it should see, such as signing in, reconnects it with `skgo.ReconnectRequestedNoArg(ctx, watchCount)` so that the new stream reads the new cookie:

```go
/// file: src/lib/auth.remote.go
// signIn opens a session and stores its id in an HttpOnly cookie. Kit allows
// cookie writes in commands and forms and nowhere else, so declaring this as
// anything but a command would make SetCookie fail.
//
// The page writes `signIn(name).updates(whoami(), getTodos(), watchCount)`.
// Everything that request asks for is named below, and nothing else is: a
// request naming a fourth query would run nothing.
func signIn(ctx context.Context, event params.RequestEvent, user string) (businesslogic.Session, error) {
	if user == "" {
		return businesslogic.Session{}, skgo.Errorf(400, "Who are you?")
	}
	id := businesslogic.Default.SignIn(user)
	if err := skgo.EventFrom(ctx).SetCookie(sessionCookie, id, skgo.CookieOptions{MaxAge: 60 * 60}); err != nil {
		return businesslogic.Session{}, err
	}
	if err := skgo.RefreshRequestedNoArg(ctx, whoami); err != nil {
		return businesslogic.Session{}, err
	}
	return businesslogic.Session{User: user}, todos.AcceptSessionRefreshes(ctx)
}
```

A command in one route package can name a query in another only through the generated import path for that directory; see [Route parameters](routing#Route-parameters).

## prerender

A `skgo.Prerender` is a query answered at build time and served as a file. `PrerenderOptions.Inputs` lists the arguments to render; see [Page options](page-options#prerender-Prerendered-remote-functions).

## Validation

Argument types are declared in Go and the generator writes them to TypeScript, so a call site that passes the wrong shape fails to type-check, and a request that does not decode into the Go type is rejected before your function runs, as a schema rejects it in Kit. There is no schema library to add. A field that may be absent is a `polytype.Optional[T]`, which tells absence from the zero value:

```go
/// file: src/routes/optional/optional.remote.go
type Input struct {
	Name    string                    `json:"name"`
	Count   polytype.Optional[int]    `json:"count,omitzero"`
	Enabled polytype.Optional[bool]   `json:"enabled,omitzero"`
	Label   polytype.Optional[string] `json:"label,omitzero"`
}
```

```go
/// file: src/routes/optional/optional.remote.go
func submit(ctx context.Context, _ params.RequestEvent, in Input) (Result, error) {
	if in.Name == "" {
		return Result{}, skgo.Invalidf("name", "A name is required")
	}
	if in.Count.Present && in.Count.Value < 0 {
		return Result{}, skgo.Invalidf("count", "Count must be zero or greater")
	}
	result := Result{Name: in.Name, Count: "absent", Enabled: "absent", Label: "absent"}
	if in.Count.Present {
		result.Count = fmt.Sprintf("present: %d", in.Count.Value)
	}
	if in.Enabled.Present {
		result.Enabled = fmt.Sprintf("present: %t", in.Enabled.Value)
	}
	if in.Label.Present {
		result.Label = fmt.Sprintf("present: %q", in.Label.Value)
	}
	operations.Lock()
	operations.byName[in.Name]++
	result.Operations = operations.byName[in.Name]
	operations.Unlock()
	return result, nil
}
```

## Using getRequestEvent

`skgo.EventFrom(ctx)` is Kit's `getRequestEvent`, available in every kind of remote function, and `event-from` in [Go and web standards](go-standards#context.Context) shows the usual helper written with it. Commands and forms also receive the generated `params.RequestEvent` as their second argument, whose `URL()` is the calling page's URL and whose `Params` are the page's route parameters, typed: `event.Params.ID()` yields an interface with one concrete type per matcher that can produce the segment, so a `switch` over it tells a string `[id]` from an `[id=Order]`. In a query, `URL()` panics and `Params` is empty, as Kit's query has neither.

## Redirects

A query that returns a `*skgo.Redirect` while a page renders redirects the whole document, as a thrown `redirect` does in Kit:

```go
/// file: src/routes/error/redirect/redirect.remote.go
func whereTo(_ context.Context) (Destination, error) {
	return Destination{}, &skgo.Redirect{Status: 307, Location: "/about"}
}
```

A form may redirect after a successful submission, as above. A command may not: Kit's wire format for a command has no room for one, so a `Redirect` returned from a command is reported as an error.
