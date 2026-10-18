package skgo

import "fmt"

// ServerInit is the application's server init hook. It runs synchronously
// while NewSSR or NativeDevelopment.NewSSR constructs an application instance,
// before the application's page, endpoint or remote handlers are returned for
// serving. This also applies when every page has ssr = false.
//
// The hook has no request event. It may perform application I/O, such as opening
// a database, and must finish before serving starts. Returning an error prevents
// construction and reports the cause to the caller. Each new application
// instance runs its own hook; requests and renderer-pool runtimes never rerun it.
// A replacement development application is a new instance and initializes again.
type ServerInit func() error

func initializeServer(init ServerInit) error {
	if init == nil {
		return nil
	}
	if err := init(); err != nil {
		return fmt.Errorf("skgo: server init failed: %w", err)
	}
	return nil
}
