package q

import (
	"context"
	kit "github.com/tylergannon/skgo"
)

func ordinary(ctx context.Context, slug string) (string, error) {
	e := kit.EventFrom(ctx)
	if err := e.SetCookie("session", slug, kit.CookieOptions{}); err != nil { // want `Kit refuses the write.*command or form`
		return "", err
	}
	if err := e.DeleteCookie("session", kit.CookieOptions{}); err != nil { // want `Kit refuses the write.*command or form`
		return "", err
	}
	_ = e.Param("slug")         // want `cache key.*typed query argument`
	_ = e.URL()                 // want `cache key.*typed query argument`
	_, _ = e.SearchParam("tab") // want `cache key.*typed query argument`
	_ = e.RouteID()             // want `cache key.*typed query argument`
	_, _ = e.Cookie("session")
	_ = e.Request().Header.Get("Accept")
	return slug, nil
}

var _ = kit.Query(ordinary)

func live(ctx context.Context, slug string, yield func(string) error) error {
	derived := context.WithValue(ctx, contextKey{}, slug)
	e := kit.EventFrom(derived)
	alias := e
	if err := alias.DeleteCookie("session", kit.CookieOptions{}); err != nil { // want `Kit refuses the write.*command or form`
		return err
	}
	_ = alias.Param("slug") // want `cache key.*typed query argument`
	return yield(slug)
}

var _ = kit.LiveQuery(live)

func batch(ctx context.Context, slugs []string) ([]string, error) {
	if err := kit.EventFrom(ctx).SetCookie("session", "x", kit.CookieOptions{}); err != nil { // want `Kit refuses the write.*command or form`
		return nil, err
	}
	_ = kit.EventFrom(ctx).RouteID() // want `cache key.*typed query argument`
	return slugs, nil
}

var _ = kit.BatchQuery(batch)

func command(ctx context.Context) (string, error) {
	e := kit.EventFrom(ctx)
	if err := e.SetCookie("session", "x", kit.CookieOptions{}); err != nil {
		return "", err
	}
	if err := e.DeleteCookie("session", kit.CookieOptions{}); err != nil {
		return "", err
	}
	return "", nil
}

var _ = kit.Command(command)

func form(ctx context.Context, arg string) (string, error) {
	if err := kit.EventFrom(ctx).SetCookie("session", arg, kit.CookieOptions{}); err != nil {
		return "", err
	}
	return arg, nil
}

var _ = kit.Form(form)

func typedPageValue(ctx context.Context, slug string) (string, error) {
	_, _ = kit.EventFrom(ctx).Cookie("session")
	return slug, nil
}

var _ = kit.Query(typedPageValue)

func shared(ctx context.Context) error {
	return kit.EventFrom(ctx).SetCookie("session", "x", kit.CookieOptions{})
}

func unknown(ctx context.Context) (string, error) {
	e := kit.EventFrom(wrap(ctx))
	if err := e.SetCookie("session", "x", kit.CookieOptions{}); err != nil {
		return "", err
	}
	return e.Param("slug"), nil
}

var _ = kit.Query(unknown)

func wrap(ctx context.Context) context.Context { return ctx }

type contextKey struct{}
