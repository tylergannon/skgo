package hotreload

// Emitted only into the private development host. The build driver and SDK
// never import the loader, whose object reader requires the prepared GOROOT.
const nativeRuntime = `
type _skgo_revision_key struct{}
func _skgo_targets_for(ctx _skgo_context.Context) *_skgo_targets {
 if ctx != nil { if targets, ok := ctx.Value(_skgo_revision_key{}).(*_skgo_targets); ok { return targets } }
 if targets := _skgo_current.Load(); targets != nil { return targets }; return _skgo_initial()
}
// SkgoHotHandler snapshots the dispatch revision for synchronous requests,
// including generated command refresh calls and serialization.
func SkgoHotHandler(next _skgo_http.Handler) _skgo_http.Handler {
 return _skgo_http.HandlerFunc(func(w _skgo_http.ResponseWriter, r *_skgo_http.Request) {
  ctx := _skgo_context.WithValue(r.Context(), _skgo_revision_key{}, _skgo_current.Load())
  next.ServeHTTP(w, r.WithContext(ctx))
 })
}
type SkgoHotHostReplacement struct { Reason string }
func (e *SkgoHotHostReplacement) Error() string { return "host replacement required: " + e.Reason }
var _skgo_mu _skgo_sync.Mutex
var _skgo_symbols map[string]uintptr
var _skgo_modules []*_skgo_loader.CodeModule
var _skgo_budget int
// SkgoHotInit registers the initialized host once, before serving requests.
func SkgoHotInit(budget int) error {
 _skgo_mu.Lock(); defer _skgo_mu.Unlock()
 if _skgo_symbols != nil || budget < 1 { return fmt.Errorf("invalid native host initialization") }
 syms := map[string]uintptr{}
 if err := _skgo_loader.RegSymbol(syms); err != nil { return err }
 _skgo_loader.RegTypes(syms, SkgoHotTypes()...)
 _skgo_symbols = syms; _skgo_budget = budget
 return nil
}
// SkgoHotInstall retains module handles and their Go-owned backing for the
// host's lifetime. Refusal never publishes a partial dispatch table.
func SkgoHotInstall(object string, storage []string) error {
 _skgo_mu.Lock(); defer _skgo_mu.Unlock()
 if _skgo_symbols == nil { return fmt.Errorf("native host is not initialized") }
 if len(_skgo_modules) >= _skgo_budget { return &SkgoHotHostReplacement{"retained revision budget exhausted"} }
 for _, name := range storage {
  if _skgo_symbols[name] == 0 { return &SkgoHotHostReplacement{"missing host storage: " + name} }
 }
 linker, err := _skgo_loader.ReadObjs([]string{object}, []string{"PACKAGE"}); if err != nil { return err }
 if missing := _skgo_loader.UnresolvedSymbols(linker, _skgo_symbols); len(missing) > 0 { return fmt.Errorf("unresolved host symbols: %v", missing) }
 _skgo_runtime.LockOSThread()
 module, err := _skgo_loader.Load(linker, _skgo_symbols)
 _skgo_runtime.UnlockOSThread()
 if err != nil { return err }
 // Retain even if a later preparation error prevents publication: loading has
 // already registered native mappings with the runtime.
 _skgo_modules = append(_skgo_modules, module)
 targets, err := _skgo_prepare(module.Syms); if err != nil { return err }
 _skgo_current.Store(targets)
 return nil
}
`
