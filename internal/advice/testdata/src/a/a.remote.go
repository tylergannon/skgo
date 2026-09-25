package a

import (
	"context"
	"github.com/tylergannon/skgo"
)

func badQuery(ctx context.Context, id string) (string, error) {
	e := skgo.EventFrom(ctx)
	_ = e.SetCookie("session", "x", skgo.CookieOptions{}) // want `command or form` `cookie write or refresh`
	e.DeleteCookie("session", skgo.CookieOptions{})       // want `command or form` `cookie write or refresh`
	_ = e.Param("id")                                     // want `cache key`
	_ = e.URL()                                           // want `cache key`
	_, _ = e.SearchParam("q")                             // want `cache key`
	_ = e.RouteID()                                       // want `cache key`
	_, _ = e.Cookie("session")
	return id, nil
}

var _ = skgo.Query(badQuery)

func goodQuery(ctx context.Context, id string) (string, error) {
	_, _ = skgo.EventFrom(ctx).Cookie("session")
	return id, nil // page value is the typed argument
}

var _ = skgo.Query(goodQuery)

func noArgQuery(ctx context.Context) (string, error) { return "value", nil }

var _ = skgo.Query(noArgQuery)

func badLive(ctx context.Context, yield func(string) error) error {
	e := skgo.EventFrom(ctx)
	_ = e.SetCookie("x", "y", skgo.CookieOptions{}) // want `command or form` `cookie write or refresh`
	yield("value")                                  // want `disconnected subscriber`
	return nil
}

var _ = skgo.LiveQuery(badLive)

func goodLive(ctx context.Context, yield func(string) error) error {
	return yield("value")
}

var _ = skgo.LiveQuery(goodLive)

func batch(ctx context.Context, ids []string) ([]string, error) {
	_ = skgo.EventFrom(ctx).Param("id") // want `cache key`
	return ids, nil
}

var _ = skgo.BatchQuery(batch)

func badContext(ctx context.Context) (string, error) {
	_ = skgo.EventFrom(context.Background())                        // want `loses.*request event`
	_ = skgo.EventFrom(context.WithValue(context.TODO(), key{}, 1)) // want `loses.*request event`
	return "", nil
}

var _ = skgo.Command(badContext)

type key struct{}

func goodContext(ctx context.Context) (string, error) {
	_ = skgo.EventFrom(ctx)
	_ = skgo.EventFrom(context.WithValue(ctx, key{}, 1))
	_ = context.Background() // intentional unrelated background work
	return "", nil
}

var _ = skgo.Command(goodContext)

func badCommand(ctx context.Context) (string, error) {
	skgo.EventFrom(ctx).SetCookie("session", "x", skgo.CookieOptions{}) // want `cookie write or refresh`
	skgo.RefreshNoArg(ctx, noArgQuery)                                  // want `cookie write or refresh`
	_ = skgo.RefreshNoArg(ctx, noArgQuery)                              // want `cookie write or refresh`
	skgo.RefreshNoArg(ctx, badCommand)                                  // want `registered Command` `cookie write or refresh`
	_ = skgo.RefreshRequestedNoArg(ctx, badCommand)                     // want `registered Command` `cookie write or refresh`
	return "", nil
}

var _ = skgo.Command(badCommand)

func goodCommand(ctx context.Context) (string, error) {
	if err := skgo.EventFrom(ctx).SetCookie("session", "x", skgo.CookieOptions{}); err != nil {
		return "", err
	}
	if err := skgo.RefreshNoArg(ctx, noArgQuery); err != nil {
		return "", err
	}
	return "", nil
}

var _ = skgo.Command(goodCommand)

type input struct {
	Email  string `json:"email"`
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
	Tags []string `json:"tags"`
}

func badForm(ctx context.Context, arg input) (string, error) {
	_ = skgo.Invalidf("emial", "bad")                // want `did you mean "email"`
	_ = (&skgo.Invalid{}).Add("author.nmae", "bad")  // want `did you mean "author.name"`
	_ = skgo.Issue{Field: "tagz[0]", Message: "bad"} // want `did you mean "tags\[0\]"`
	_ = skgo.Invalidf("email.part", "bad")           // want `not in the declared input`
	_ = skgo.Invalidf("tags[0].part", "bad")         // want `not in the declared input`
	return "", nil
}

var _ = skgo.Form(badForm)

func goodForm(ctx context.Context, arg input) (string, error) {
	_ = skgo.Invalidf("", "whole form")
	_ = skgo.Invalidf("email", "bad")
	_ = skgo.Invalidf("author.name", "bad")
	_ = skgo.Invalidf("tags[0]", "bad")
	field := "computed"
	_ = skgo.Invalidf(field, "bad")
	return "", nil
}

var _ = skgo.Form(goodForm)
