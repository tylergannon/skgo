package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedCommandFormCallerEvents(t *testing.T) {
	app, cfg := sharedFixture(t)
	for _, route := range []string{"choice/[id=Numeric]", "choice/[id]", "chain/[[lang=Lang]]/[[id]]"} {
		writeSharedFixture(t, app, "web/src/routes/"+route+"/+page.svelte", "<p>caller</p>")
	}
	path := filepath.Join(app, "web/src/params.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source = append(source, []byte("\nvar LangCalls int\nfunc Lang(s string)(string,bool){LangCalls++;return s,s==\"en\"}\n")...)
	source = []byte(strings.Replace(string(source), `if s=="none" {return nil,true}`, `if s=="typednil" {return (*domain.OrderRef)(nil),true};if s=="none" {return nil,true}`, 1))
	if err := os.WriteFile(path, source, 0644); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(app, "web/src/params.ts")
	source, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source = []byte(strings.Replace(string(source), "defineParams({", "defineParams({ Lang:(value:string)=>value===\"en\"?value:undefined,", 1))
	if err := os.WriteFile(path, source, 0644); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "web/src/caller/caller.remote.go", callerRemoteSource)
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "internal/skgo/caller_test.go", callerHandlerTests)
	cmd := exec.Command("go", "test", "-race", "-count=1", "-v", "-run", "^TestCaller", "./internal/skgo")
	cmd.Dir = app
	output, err := cmd.CombinedOutput()
	if err != nil || strings.Contains(string(output), "SKIP") || !strings.Contains(string(output), "--- PASS: TestCallerHandlers") || !strings.Contains(string(output), "--- PASS: TestCallerOverlap") {
		t.Fatalf("generated handler contracts did not pass: %v\n%s", err, output)
	}
	t.Logf("generated handler contracts:\n%s", output)
}

func TestCommandFormContextMigrationDiagnostics(t *testing.T) {
	for _, kind := range []string{"Command", "Form"} {
		for _, tc := range []struct{ name, signature, want string }{
			{"ctx-only", "ctx context.Context,in string", "A " + strings.ToLower(kind) + " is func(context.Context, skgo.RequestEvent[params.Params]"},
			{"event-only", "event skgo.RequestEvent[params.Params],in string", "A " + strings.ToLower(kind) + " is func(context.Context, skgo.RequestEvent[params.Params]"},
			{"wrong-params", "ctx context.Context,event skgo.RequestEvent[struct{}],in string", "must receive"},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				_, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo";"example.com/app/generated/params")
var _ = params.Params{}; var _ = context.Background
func old(`+tc.signature+`)(string,error){return in,nil}
var _ = skgo.`+kind+`(old)
`, nil)
				err := Run(cfg)
				if err == nil || !strings.Contains(err.Error(), "data.remote.go:5:") || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("missing source-located %s migration: %v", kind, err)
				}
			})
		}
	}
}

const callerRemoteSource = `package caller
import (
 "context"
 "fmt"
 "reflect"
 "time"
 "github.com/tylergannon/skgo"
 "github.com/tylergannon/skgo/example/internal/skgo/params"
)
type Input struct { Name string ` + "`json:\"name\"`" + ` }
type ContextKey struct{}
var Enter = make(chan struct{},2)
var Release = make(chan struct{})
func forbidden(fn func())(threw bool){defer func(){threw=recover()!=nil}();fn();return}
func receipt(p params.Params) string {
 if p.Lang()!=nil {panic("rejected optional Lang getter must be absent")}
 if v:=p.ID();v!=nil {
  if reflect.TypeOf(v).Kind()!=reflect.Struct {panic("pointer wrapper")}
  switch id:=v.(type){
  case params.IDParam_DomainNumber_f2699d13762bca7d8b7b8d63120233e8bd9a519b8afaa1e5fedbb0bc340de94a:return "numeric:"+id.Value.Label()
  case params.IDParam_String:return "string:"+id.Value
  case params.IDParam_Ref:
   if id.Value==nil{return "remote:present:nil"}
   if reflect.ValueOf(id.Value).IsNil(){return "remote:present:typednil"}
   return "remote:present:"+id.Value.Label()
  default:panic(fmt.Sprintf("unknown id %T",v))
  }
 }
 if v:=p.Pointer();v!=nil {switch v:=v.(type){case params.PointerParam_PointerOrderRef:if v.Value!=nil{panic("pointer payload")};return "pointer:present:nil";default:panic("pointer variant")}}
 if v:=p.Flag();v!=nil {switch v:=v.(type){case params.FlagParam_Bool:return fmt.Sprintf("bool:%t",v.Value);default:panic("flag variant")}}
 if v:=p.Empty();v!=nil {switch v:=v.(type){case params.EmptyParam_String:return "empty:"+v.Value;default:panic("empty variant")}}
 if v:=p.Order();v!=nil {switch v:=v.(type){
 case params.OrderParam_DomainOrder:return fmt.Sprintf("struct:%d:%s",v.Value.Number,v.Value.Label())
 case params.OrderParam_LegacyOrder:return fmt.Sprintf("legacy-struct:%d:%s",v.Value.Number,v.Value.Label())
 default:panic("order variant")}}
 return "remote:absent"
}
func check(ctx context.Context,event skgo.RequestEvent[params.Params]) {
 for _,fn:=range []func(){func(){skgo.EventFrom(ctx).Param("id")},func(){skgo.EventFrom(ctx).Params()},func(){skgo.EventFrom(ctx).URL()},func(){skgo.EventFrom(ctx).RouteID()},func(){params.SkgoRequestEvent(skgo.EventFrom(ctx))},func(){event.Event.Param("id")},func(){event.Event.Params()},func(){skgo.EventFrom(event.Context()).Param("id")},func(){skgo.EventFrom(event.Context()).Params()},func(){skgo.EventFrom(event.Context()).URL()},func(){skgo.EventFrom(event.Context()).RouteID()},func(){params.SkgoRequestEvent(skgo.EventFrom(event.Context()))}} {
  if !forbidden(fn){panic("raw/context caller access allowed")}
 }
 if ctx.Value(ContextKey{}) != "request-value" {panic("ctx lost request value")}
 if deadline,ok:=ctx.Deadline();!ok || !deadline.Equal(time.Unix(4102444800,0)) {panic("ctx lost deadline")}
 if ctx.Err()!=context.Canceled {panic("ctx lost cancellation")}
 cookie,ok:=skgo.EventFrom(ctx).Cookie("request-cookie");if !ok || cookie!="from-request" {panic("ctx lost cookies")}
 if err:=skgo.EventFrom(ctx).SetCookie("response-cookie","from-handler",skgo.CookieOptions{});err!=nil {panic(err)}
 if cookie,ok:=event.Cookie("response-cookie");!ok || cookie!="from-handler" {panic("ctx and event have different cookie jars")}
 if err:=skgo.RefreshNoArg(ctx,nested);err!=nil {panic(err)}
 // A direct nested query gets the restricted context too.
 if got,_:=nested(ctx);got!="query:restricted"{panic(got)}
}
func act(ctx context.Context, event skgo.RequestEvent[params.Params],input string)(string,error){
 check(ctx,event)
 if input=="hold" { Enter<-struct{}{};<-Release }
 return input+"|"+receipt(event.Params),nil
}
type EventAlias = skgo.RequestEvent[params.Params]
func noInput(ctx context.Context, event EventAlias)(string,error){check(ctx,event);return "no-input|"+receipt(event.Params),nil}
func submit(ctx context.Context, event skgo.RequestEvent[params.Params],input Input)(string,error){if input.Name!="go-client"{check(ctx,event)};if input.Name=="hold" {Enter<-struct{}{};<-Release};return input.Name+"|"+receipt(event.Params),nil}
` + `
func nested(ctx context.Context)(string,error){
 e:=skgo.EventFrom(ctx)
 for _,fn:=range []func(){func(){e.Param("id")},func(){e.Params()},func(){e.URL()},func(){e.RouteID()},func(){params.SkgoRequestEvent(e)}} {if !forbidden(fn){panic("query leaked caller")}}
 return "query:restricted",nil
}
var (_=skgo.Command(act);_=skgo.Command(noInput);_=skgo.Form(submit);_=skgo.Query(nested))
`

const callerHandlerTests = `package skgo_test
import (
 "bytes"
 "context"
 "encoding/json"
 "encoding/binary"
 "encoding/base64"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync"
 "testing"
 "time"
 "github.com/tylergannon/skgo"
 "github.com/tylergannon/polytype/devalue"

 generated "github.com/tylergannon/skgo/example/internal/skgo"
 client "github.com/tylergannon/skgo/example/internal/skgo/client"
 caller "github.com/tylergannon/skgo/example/web/src/caller"
 matchers "github.com/tylergannon/skgo/example/web/src"
)
// These are literal Kit route patterns and parameter descriptors. The registry
// has neither Loads nor a handle hook: frontend-only callers must still match.
func registry(t *testing.T,dev bool)(*skgo.Remotes,map[string]*skgo.Remote){t.Helper()
 routes:=[]skgo.ManifestRoute{
 {ID:"/numeric/[id=Numeric]",Pattern:"^/numeric/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"id",Matcher:"Numeric"}}},
 {ID:"/text/[id]",Pattern:"^/text/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"id"}}},
 {ID:"/optional/[[id]]",Pattern:"^/optional(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"id",Optional:true,Chained:true}}},
 {ID:"/absent",Pattern:"^/absent/?$"},
 {ID:"/nil/[id=MaybeRef]",Pattern:"^/nil/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"id",Matcher:"MaybeRef"}}},
 {ID:"/nil-optional/[[id=MaybeRef]]",Pattern:"^/nil-optional(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"id",Matcher:"MaybeRef",Optional:true,Chained:true}}},
 {ID:"/pointer/[[pointer=Pointer]]",Pattern:"^/pointer(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"pointer",Matcher:"Pointer",Optional:true,Chained:true}}},
 {ID:"/flag/[flag=Flag]",Pattern:"^/flag/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"flag",Matcher:"Flag"}}},
 {ID:"/empty/[empty=Blank]",Pattern:"^/empty/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"empty",Matcher:"Blank"}}},
 {ID:"/named/[order=StructOrder]",Pattern:"^/named/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"order",Matcher:"StructOrder"}}},
 {ID:"/legacy/[order=LegacyOrder]",Pattern:"^/legacy/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"order",Matcher:"LegacyOrder"}}},
 {ID:"/choice/[id=Numeric]",Pattern:"^/choice/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"id",Matcher:"Numeric"}}},
 {ID:"/choice/[id]",Pattern:"^/choice/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"id"}}},
 {ID:"/chain/[[lang=Lang]]/[[id]]",Pattern:"^/chain(?:/([^/]+))?(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"lang",Matcher:"Lang",Optional:true,Chained:true},{Name:"id",Optional:true,Chained:true}}},
 }
 remotes:=generated.Remotes();byName:=map[string]*skgo.Remote{}
 for _,r:=range remotes{if r.Module()=="src/caller/caller.remote.ts" {byName[r.Name()]=r}}
 rs,err:=skgo.NewRemotes(skgo.RemoteConfig{Dev:dev,Routes:routes},remotes...);if err!=nil{t.Fatal(err)}
 return rs,byName
}
func dispatch(rs *skgo.Remotes,fn *skgo.Remote,path,input string)*httptest.ResponseRecorder{
 var body []byte;media:="application/json"
 if fn.Kind()==skgo.KindForm {
  // Enhanced endpoint, real binary form decoder and generated DecodeForm.
  header:=fmt.Sprintf("[[1,3],{\"name\":2},%q,{}]",input)
  body=make([]byte,7+len(header));binary.LittleEndian.PutUint32(body[1:5],uint32(len(header)));copy(body[7:],header)
  media="application/x-sveltekit-formdata"
 }else{
  payload:=""
  if fn.Name()!="noInput" {payload=base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("[%q]",input)))}
  body,_=json.Marshal(map[string]any{"payload":payload,"refreshes":[]any{}})
 }
 req:=httptest.NewRequest("POST",rs.Prefix()+fn.ID(),bytes.NewReader(body));req.Header.Set("Content-Type",media)
 if path!=""{req.Header.Set("x-sveltekit-pathname",path)}
 ctx,cancel:=context.WithDeadline(context.WithValue(req.Context(),caller.ContextKey{},"request-value"),time.Unix(4102444800,0));cancel()
 req=req.WithContext(ctx);req.AddCookie(&http.Cookie{Name:"request-cookie",Value:"from-request"})
 rec:=httptest.NewRecorder();rs.ServeHTTP(rec,req);return rec
}
func result(rec *httptest.ResponseRecorder,form bool)(string,error){
 var env struct{Type,Data string;Error any};if err:=json.Unmarshal(rec.Body.Bytes(),&env);err!=nil{return "",err}
 if rec.Code!=200 || env.Type!="result"{return "",fmt.Errorf("%d %s",rec.Code,rec.Body.String())}
 tree,err:=devalue.Parse(env.Data,nil);if err!=nil{return "",err}
 root:=tree.(*devalue.Object);value,_:=root.Get("_")
 if form {value,_=root.Get("_");value,_=value.(*devalue.Object).Get("result")}
 got,ok:=value.(string);if !ok{return "",fmt.Errorf("result %T: %s",value,env.Data)};return got,nil
}
func TestCallerHandlers(t *testing.T){
 for _,dev:=range []bool{false,true}{t.Run(fmt.Sprintf("dev=%t",dev),func(t *testing.T){
 matchers.LangCalls=0
 rs,fns:=registry(t,dev)
 for _,tc:=range []struct{path,want string}{
 {"/numeric/42","numeric:Order #42"},{"/numeric/0","numeric:Order #0"},{"/text/42","string:42"},{"/absent","remote:absent"},{"/optional","remote:absent"},{"/optional/hi","string:hi"},
 {"/choice/42","numeric:Order #42"},{"/choice/word","string:word"},{"/choice/0","numeric:Order #0"},
 {"/nil/none","remote:present:nil"},{"/nil/42","remote:present:Ref #42"},{"/nil/typednil","remote:present:typednil"},
 {"/nil-optional/none","remote:present:nil"},{"/nil-optional","remote:absent"},{"/pointer/none","pointer:present:nil"},{"/pointer","remote:absent"},
 {"/flag/false","bool:false"},{"/empty/ignored","empty:"},{"/named/42","struct:42:Struct order #42"},
 {"/legacy/17","legacy-struct:17:Legacy order #17"},
 {"","remote:absent"},{"/unmatched","remote:absent"},{"/numeric/rejected","remote:absent"},
 {"/text/%2525","string:%25"},{"/text/a%2Fb","string:a/b"},{"/text/%E2%9C%93","string:✓"},
 {"/chain/abc","string:abc"},
 }{
  for _,name:=range []string{"act","submit","noInput"}{
   prefix:=map[string]string{"act":"command-input","submit":"form-input","noInput":"no-input"}[name]
   got,err:=result(dispatch(rs,fns[name],tc.path,prefix),name=="submit")
   if err!=nil || got!=prefix+"|"+tc.want{t.Fatalf("%s caller %q: %q %v; want %q",name,tc.path,got,err,prefix+"|"+tc.want)}
  }
 }
 if matchers.LangCalls!=3{t.Fatalf("chained optional matcher calls %d, want 3",matchers.LangCalls)}
 for _,name:=range []string{"act","submit"}{rec:=dispatch(rs,fns[name],"/text/%ZZ","malformed");if rec.Code!=400{t.Fatalf("malformed %s: %d %s",name,rec.Code,rec.Body.String())}}
 rec:=httptest.NewRecorder();rs.ServeHTTP(rec,httptest.NewRequest("POST",rs.Prefix()+"unknown/remote",strings.NewReader("{}")))
 if !strings.Contains(rec.Body.String(),` + "`" + `"status":404` + "`" + `){t.Fatalf("unknown remote: %d %s",rec.Code,rec.Body.String())}
 // Generated Go form client has no caller headers; a known form still runs.
 server:=httptest.NewServer(rs);defer server.Close()
 c:=client.Client{FormClient:skgo.FormClient{BaseURL:server.URL}}
 got,err:=c.Submit(context.Background(),caller.Input{Name:"go-client"})
 if err!=nil || got!="go-client|remote:absent"{t.Fatalf("Go client: %q %v",got,err)}
 })}
}
func TestCallerOverlap(t *testing.T){
 for _,dev:=range []bool{false,true}{t.Run(fmt.Sprintf("dev=%t",dev),func(t *testing.T){
 for _,name:=range []string{"act","submit"}{t.Run(name,func(t *testing.T){
 rs,fns:=registry(t,dev)
 caller.Enter=make(chan struct{},2);caller.Release=make(chan struct{})
 var wg sync.WaitGroup;results:=make(chan string,2)
 for _,path:=range []string{"/numeric/42","/text/word"}{wg.Add(1);go func(){defer wg.Done();got,err:=result(dispatch(rs,fns[name],path,"hold"),name=="submit");if err!=nil{got=err.Error()};results<-got}()}
 for range 2{select{case <-caller.Enter:case <-time.After(5*time.Second):t.Fatal("callbacks did not overlap")}}
 close(caller.Release);wg.Wait();close(results)
 got:=map[string]int{};for value:=range results{got[value]++}
 if got["hold|numeric:Order #42"]!=1 || got["hold|string:word"]!=1{t.Fatalf("overlap receipts %v",got)}
 })}
 })}
}
`
