package hotreload

// Private to generated main, where the application constructor transfers its
// handler to one generation. NativeDevelopment owns its logical requests, whose
// goroutines may outlive the HTTP calls which created them.
const hostHandlerRuntime = `
type handlerGeneration struct {
 handler, owner http.Handler
 bridgeClose func()
 mu sync.Mutex
 users int
 retired bool
 idle chan struct{}
}
func closeApplication(owner http.Handler) {
 if closer,ok:=owner.(io.Closer);ok {
  if err:=closer.Close();err!=nil{fmt.Fprintln(os.Stderr,"close development application:",err)}
 }
}
func(g *handlerGeneration) acquire(){g.mu.Lock();g.users++;g.mu.Unlock()}
func(g *handlerGeneration) release(){
 g.mu.Lock();defer g.mu.Unlock()
 g.users--
 if g.retired && g.users==0{close(g.idle)}
}
func(g *handlerGeneration) retire(owners *sync.WaitGroup){
 g.mu.Lock()
 g.retired=true
 idle:=g.users==0
 if idle{close(g.idle)}
 g.mu.Unlock()
 cleanup:=func(){
  defer owners.Done()
  <-g.idle
  // First end and join the bridge's logical requests and fetch hooks. Only
  // then can application resources captured by those goroutines be closed.
  if g.bridgeClose!=nil{g.bridgeClose()}
  closeApplication(g.owner)
 }
 // Idle replacement settles before acknowledgement. A held HTTP request
 // drains asynchronously, so it cannot block the control loop. The new
 // generation is already published while native logical work is joined.
 if idle {cleanup()} else {go cleanup()}
}
`
