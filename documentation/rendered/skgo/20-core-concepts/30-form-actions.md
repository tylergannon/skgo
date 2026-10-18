---
title: "Form actions"
summary: "Page actions written in Go, with validation failures, redirects and progressive enhancement."
---

A `page.server.go` can declare [actions](https://svelte.dev/docs/kit/form-actions), which let a `<form>` `POST` data to the server. They work without client-side JavaScript, and `use:enhance` improves them when it is available, exactly as in Kit. The action itself is a Go function.

> [!NOTE] Kit's note applies here too: the [`form` remote function](remote-functions#form) covers the same ground with type safety and single-flight mutations, and new development is focused there. Page actions are complete and keep working.

## Default actions

The simplest page declares one default action with `skgo.DefaultAction`:

```go
/// file: src/routes/actions/default/page.server.go
package defaultaction

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/actiondemo"
)

type SaveResult struct {
	Receipt   string `json:"receipt"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Biography string `json:"biography"`
	State     string `json:"state"`
}

func save(ctx context.Context) (SaveResult, error) {
	id, err := actiondemo.Visitor(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := actiondemo.SaveDefault(id, actiondemo.Profile{
		Name: r.PostForm.Get("name"), Email: r.PostForm.Get("email"),
		Biography: r.PostForm.Get("biography"), State: "Active",
	})
	return SaveResult{Receipt: "Saved " + profile.Name, Name: profile.Name,
		Email: profile.Email, Biography: profile.Biography, State: profile.State}, nil
}

var _ = skgo.DefaultAction(save)
```

```svelte
<!--- file: src/routes/actions/default/+page.svelte --->
<form method="POST">
	<label>Name <input name="name" /></label>
	<label>Email <input name="email" /></label>
	<label>Biography <textarea name="biography"></textarea></label>
	<button>Save</button>
</form>
```

Submitting the form `POST`s it to the page and runs the Go function. An action always answers a `POST`; as in Kit, a `GET` never has side effects.

## Named actions

Instead of a default action, a page can declare as many named actions as it needs. The action's name is the Go function's name:

```go
/// file: src/routes/actions/page.server.go
var _ = skgo.ActionWithFailure(save, ValidationFailure{})
var _ = skgo.Action(upload)
var _ = skgo.Action(inspect)
var _ = skgo.Action(remember)
var _ = skgo.Action(remote)
var _ = skgo.ActionNoData(archive)
var _ = skgo.ActionNoData(signIn)
var _ = skgo.ActionNoData(forbidden)
var _ = skgo.ActionNoData(unavailable)
```

A form picks one with `action="?/save"`, or a button with `formaction="?/archive"`, as in Kit:

```svelte
<!--- file: src/routes/actions/+page.svelte --->
<form method="POST" action="?/save">
	<!-- fields -->
	<button>Save</button>
	<button formaction="?/archive">Archive</button>
</form>
```

Kit's restriction holds: a page cannot have a default action next to named ones.

## Anatomy of an action

An action is `func(ctx context.Context) (T, error)`. The posted form is on the request, which `skgo.EventFrom(ctx).Request()` returns; parse it with the standard library and read `PostForm`. What the function returns is available to the page as `form`, and app-wide as `page.form`, until the next update:

```go
/// file: src/routes/actions/page.server.go
func save(ctx context.Context) (SaveResult, error) {
	id, err := workspace(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := Profile{
		Name:      request.PostForm.Get("name"),
		Email:     request.PostForm.Get("email"),
		Biography: request.PostForm.Get("biography"),
		State:     "Active",
	}
	address, parseErr := mail.ParseAddress(profile.Email)
	if parseErr != nil || address.Address != profile.Email || !strings.Contains(strings.SplitAfter(profile.Email, "@")[1], ".") {
		return SaveResult{}, skgo.Fail(http.StatusUnprocessableEntity, ValidationFailure{
			Name: profile.Name, Email: profile.Email, Biography: profile.Biography,
			EmailError: "Enter a valid email address",
		})
	}
	profiles.Lock()
	profiles.byWorkspace[id] = profile
	profiles.Unlock()
	if err := skgo.EventFrom(ctx).SetHeader("x-skgo-action-demo", "profile-saved"); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Receipt: "Saved " + profile.Name, Price: businesslogic.Money{Cents: 750}}, nil
}
```

Three markers declare what an action returns. `skgo.Action(fn)` is a function returning `(T, error)`; `skgo.ActionNoData(fn)` returns only `error`, for an action with nothing to say; `skgo.ActionWithFailure(fn, F{})` returns `(T, error)` and may fail with an `F`. The example's `save` uses the last one.

An action may set cookies with `event.SetCookie`, set response headers with `event.SetHeader`, and read uploaded files with `ParseMultipartForm` and `FormFile`; see [Form data and files](go-standards#Form-data-and-files).

### Validation errors

When the submission cannot be processed, return `skgo.Fail(status, F{...})` with the fields the visitor typed, so the page can show them again. `F` is the type named in `skgo.ActionWithFailure`, and the page's `form` is typed as the union of the success type and the failure type:

```go
/// file: src/routes/actions/page.server.go
type ValidationFailure struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Biography  string `json:"biography"`
	EmailError string `json:"emailError"`
}
```

```svelte
<!--- file: src/routes/actions/+page.svelte --->
<script lang="ts">
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	let rejected = $derived(form && 'emailError' in form ? form : null);
</script>

<form method="POST" action="?/save">
	<label>
		Name
		<input name="name" value={rejected ? rejected.name : data.profile.name} />
	</label>
	<label>
		Email
		<input name="email" value={rejected ? rejected.email : data.profile.email} />
	</label>
	{#if rejected}<p class="error">{rejected.emailError}</p>{/if}
	<button>Save</button>
</form>
```

With `use:enhance`, or a request that accepts JSON, the response status is the one passed to `Fail`; a successful action with data answers 200 and one with no data answers 204, as in Kit. As a precaution, return only what the page needs to redraw: `save` returns the email but never a password.

### Redirects

Return a `*skgo.Redirect` to send the visitor elsewhere after the action, typically with a 303:

```go
/// file: src/routes/actions/page.server.go
func signIn(ctx context.Context) error {
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	if request.PostForm.Get("username") != "ada" {
		return skgo.Errorf(http.StatusBadRequest, "Unknown demo user")
	}
	if err := skgo.EventFrom(ctx).SetCookie(signInCookie, "ada", skgo.CookieOptions{Path: "/actions"}); err != nil {
		return err
	}
	return &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in"}
}
```

An expected error is `skgo.Errorf(status, message)`, which renders the nearest `+error.svelte` with that status; any other error is a 500 whose message is what the `handleError` hook chose. See [Errors](load#Errors).

```go
/// file: src/routes/actions/page.server.go
func forbidden(context.Context) error {
	return skgo.Errorf(http.StatusForbidden, "You cannot edit this profile")
}
```

## Loading data

After an action runs, the page's loads run again and the page re-renders with the action's result as `form`, unless the action redirected or failed unexpectedly. The `handle` hook runs before the action and not again before the loads, so an action that signs the visitor in or out must update `event.Locals` as well as the cookie, as Kit's page warns, or the loads that follow will read the old session.

## Route parameters

An action on a page under `[profile]` reads the segment with `skgo.EventFrom(ctx).Param("profile")`:

```go
/// file: src/routes/actions/profiles/[profile]/page.server.go
func save(ctx context.Context) (SaveResult, error) {
	name, err := selected(skgo.EventFrom(ctx).Param("profile"))
	if err != nil {
		return SaveResult{}, err
	}
	id, err := actiondemo.Visitor(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := actiondemo.Profile{Name: r.PostForm.Get("name"), Email: r.PostForm.Get("email"), Biography: r.PostForm.Get("biography"), State: "Active"}
	if !actiondemo.SaveSelected(id, name, profile) {
		return SaveResult{}, skgo.Errorf(http.StatusNotFound, "Profile not found")
	}
	return SaveResult{Receipt: "Saved " + profile.Name}, nil
}
```

## Progressive enhancement

Nothing here changes. Without JavaScript the browser posts the form and Go answers with the re-rendered document; with `use:enhance` from `$app/forms`, Kit's client posts it and applies the result without a full reload, following every rule in Kit's [progressive enhancement](https://svelte.dev/docs/kit/form-actions#Progressive-enhancement) section, including `applyAction`, `update`, cross-page submissions and custom event listeners.

```svelte
<!--- file: src/routes/actions/+page.svelte --->
<script lang="ts">
	import { enhance } from '$app/forms';
</script>

<form method="POST" action="?/save" use:enhance>
	<!-- fields -->
</form>
```

The example's scenarios run every action twice, once natively and once enhanced, and expect the same receipt from both.

## Alternatives

A `server.go` beside the page can expose the same operation as an HTTP API, and Kit's [content negotiation](routing#server-Content-negotiation) decides which answers a `POST`: a browser form reaches the action, a `fetch` with a JSON `accept` header reaches the endpoint. As in Kit, a `fetch` that must reach the action instead sends the `x-sveltekit-action: true` header.

```go
/// file: src/routes/actions/server.go
package actions

import (
	"encoding/json"
	"net/http"

	"github.com/tylergannon/skgo"
)

// This sibling +server route answers a POST only when Kit selects the endpoint.
func endpointPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"answer": "Endpoint POST answered by Go"})
}

var _ = skgo.POST(endpointPost)
```

## GET vs POST

Unchanged. A form with `method="GET"` navigates to its `action` URL through Kit's client router and runs your Go loads with the new query string; it never runs an action. [Using URL data](load#Using-URL-data) shows a load reading that query string.
