package hotreload

// Emitted only into the private development host. The build driver and SDK
// never import the loader, whose object reader requires the prepared GOROOT.
const nativeRuntime = `
type _skgo_revision_key struct{}
type _skgo_revision struct {
 module *_skgo_loader.CodeModule
 consumers int
 reclaim, retired bool
}
// Acquisition shares publication's lock so no reader can observe a retired,
// unpinned target between loading the dispatch table and claiming ownership.
func _skgo_acquire(ctx _skgo_context.Context) (*_skgo_targets, func()) {
 if ctx != nil { if targets, ok := ctx.Value(_skgo_revision_key{}).(*_skgo_targets); ok && targets != nil { return targets, func(){} } }
 _skgo_mu.Lock()
 targets := _skgo_current.Load()
 revision := targets._skgo_revision
 if revision != nil { revision.consumers++ }
 _skgo_mu.Unlock()
 return targets, func(){
  if revision == nil { return }
  _skgo_mu.Lock(); defer _skgo_mu.Unlock()
  revision.consumers--
  _skgo_retire(revision)
 }
}
// The outer handler owns consumption through response serialization/writing.
// Nested generated calls borrow its revision. Direct stable calls acquire their
// own synchronous lease; only the positive numeric class can be reclaimed.
func SkgoHotHandler(next _skgo_http.Handler) _skgo_http.Handler {
 return _skgo_http.HandlerFunc(func(w _skgo_http.ResponseWriter, r *_skgo_http.Request) {
  targets, release := _skgo_acquire(r.Context()); defer release()
  ctx := _skgo_context.WithValue(r.Context(), _skgo_revision_key{}, targets)
  next.ServeHTTP(w, r.WithContext(ctx))
 })
}
func _skgo_retire(revision *_skgo_revision) {
 if !revision.retired || !revision.reclaim || revision.consumers != 0 { return }
 revision.module.Unload()
 for i, loaded := range _skgo_modules { if loaded == revision {
  _skgo_modules = append(_skgo_modules[:i], _skgo_modules[i+1:]...)
  break
 } }
}
type SkgoHotHostReplacement struct { Reason string }
func (e *SkgoHotHostReplacement) Error() string { return "host replacement required: " + e.Reason }
var _skgo_mu _skgo_sync.Mutex
var _skgo_symbols map[string]uintptr
var _skgo_modules []*_skgo_revision
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
// SkgoHotInstall publishes one coherent table. Reclaim is set only by the
// compiler's positive numeric class; compatibility by itself retains backing.
func SkgoHotInstall(object string, storage []string, reclaim ...bool) error {
 _skgo_mu.Lock(); defer _skgo_mu.Unlock()
 if _skgo_symbols == nil { return fmt.Errorf("native host is not initialized") }
 if len(_skgo_modules) >= _skgo_budget { return &SkgoHotHostReplacement{"retained revision budget exhausted"} }
 for _, name := range storage {
  if _skgo_symbols[name] == 0 { return &SkgoHotHostReplacement{"missing host storage: " + name} }
 }
 linker, err := _skgo_loader.ReadObjs([]string{object}, []string{"PACKAGE"}); if err != nil { return err }
 if missing := _skgo_loader.UnresolvedSymbols(linker, _skgo_symbols); len(missing) > 0 { return fmt.Errorf("unresolved host symbols: %v", missing) }
 linker.Reclaimable = len(reclaim) == 1 && reclaim[0]
 _skgo_runtime.LockOSThread()
 module, err := _skgo_loader.Load(linker, _skgo_symbols)
 _skgo_runtime.UnlockOSThread()
 if err != nil { return err }
 // Retain uncertain preparation even when publication cannot proceed.
 revision := &_skgo_revision{module:module}
 _skgo_modules = append(_skgo_modules, revision)
 targets, err := _skgo_prepare(module.Syms); if err != nil { return err }
 revision.reclaim = len(reclaim) == 1 && reclaim[0]
 targets._skgo_revision = revision
 previous := _skgo_current.Swap(targets)
 if previous._skgo_revision != nil {
  previous._skgo_revision.retired = true
  _skgo_retire(previous._skgo_revision)
 }
 return nil
}
`
