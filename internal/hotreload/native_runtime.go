package hotreload

// Emitted only into the private development host. The build driver and SDK
// never import the loader, whose object reader requires the prepared GOROOT.
const nativeRuntime = `
const _skgo_build_identity = "SKGO_HOST_IDENTITY"
const _skgo_dependency_identity = "SKGO_HOST_DEPENDENCIES"
type _skgo_artifact struct { Version, OS, Arch, Identity, Package, SHA256, Dependencies, CompilerProfile string }
func _skgo_check_artifact(object string) error {
 raw, err := _skgo_os.ReadFile(object+".json"); if err != nil { return fmt.Errorf("native object profile: %w",err) }
 var artifact _skgo_artifact
 if err := _skgo_json.Unmarshal(raw,&artifact); err != nil { return fmt.Errorf("native object profile: %w",err) }
 if artifact.CompilerProfile != "go1.27-native-v1" || artifact.Version != _skgo_runtime.Version() || artifact.OS != _skgo_runtime.GOOS || artifact.Arch != _skgo_runtime.GOARCH || artifact.Package != "PACKAGE" || (_skgo_build_identity != "SKGO_HOST_IDENTITY" && artifact.Identity != _skgo_build_identity) || (_skgo_dependency_identity != "SKGO_HOST_DEPENDENCIES" && artifact.Dependencies != _skgo_dependency_identity) {
  return fmt.Errorf("native host/object profile mismatch: object=%+v host=%s %s/%s",artifact,_skgo_runtime.Version(),_skgo_runtime.GOOS,_skgo_runtime.GOARCH)
 }
 body, err := _skgo_os.ReadFile(object); if err != nil { return err }
 if fmt.Sprintf("%x",_skgo_sha256.Sum256(body)) != artifact.SHA256 { return fmt.Errorf("native object content does not match compiler artifact: %s",object) }
 return nil
}
type _skgo_revision_key struct{}
type _skgo_revision struct {
 module *_skgo_loader.CodeModule
 consumers int
 reclaim, retired bool
}
// Acquisition shares publication's lock so no reader can observe a retired,
// unpinned target between loading the dispatch table and claiming ownership.
// SkgoHotContext carries only the native table through the SDK's fresh
// subrequest boundary. The nested handler takes its own consumer ownership.
func SkgoHotContext(dst,src _skgo_context.Context) _skgo_context.Context {
 if targets,ok:=src.Value(_skgo_revision_key{}).(*_skgo_targets);ok && targets!=nil{return _skgo_context.WithValue(dst,_skgo_revision_key{},targets)}
 return dst
}
func _skgo_acquire(ctx _skgo_context.Context, handler ...bool) (*_skgo_targets, func()) {
 var targets *_skgo_targets
 if ctx != nil { targets,_ = ctx.Value(_skgo_revision_key{}).(*_skgo_targets) }
 if targets!=nil && len(handler)==0 {return targets,func(){}}
 _skgo_mu.Lock()
 if targets==nil {targets = _skgo_current.Load()}
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
  targets, release := _skgo_acquire(r.Context(),true); defer release()
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
// SkgoHotUsage reports charged revisions and actual active consumers to the
// owning development session. Retained backing remains charged after release.
func SkgoHotUsage()(int,int){
 _skgo_mu.Lock();defer _skgo_mu.Unlock()
 consumers:=0;for _,revision:=range _skgo_modules{consumers+=revision.consumers}
 return len(_skgo_modules),consumers
}
// SkgoHotInit registers the initialized host once, before serving requests.
func SkgoHotInit(budget int) error {
 _skgo_mu.Lock(); defer _skgo_mu.Unlock()
 if _skgo_symbols != nil || budget < 1 { return fmt.Errorf("invalid native host initialization") }
 syms := map[string]uintptr{}
 if err := _skgo_loader.RegSymbol(syms); err != nil { return fmt.Errorf("native host symbols unavailable; build with -ldflags=-s=false: %w",err) }
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
 if err := _skgo_check_artifact(object); err != nil { return err }
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
