package app

type Locals struct {
	Value        string
	Calls        int
	AssetsReady  bool
	HeadersReady bool
}

// SerializedHeader is shared by the build service and the runtime renderer.
func SerializedHeader(name, value string) bool { return name == "x-default" }
