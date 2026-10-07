package gen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeSharedFixture(t *testing.T, app, path, source string) {
	t.Helper()
	path = filepath.Join(app, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}

func sharedFixture(t *testing.T) (string, Config) {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "domain/domain.go", `package domain
import "fmt"
type Number int
func (n Number) Label() string { return fmt.Sprintf("Order #%d",n) }
type NumberAlias = Number
type Order struct { Number int }
func (r Order) Label() string { return fmt.Sprintf("Struct order #%d",r.Number) }
type OrderRef struct { Number int }
func (r *OrderRef) Label() string { return fmt.Sprintf("Ref #%d",r.Number) }
type Ref interface { Label() string }
type Box[T any] struct { Value T }
type hidden int
type PublicHidden = hidden
`)
	writeSharedFixture(t, app, "other/domain.go", `package domain
type Number int
`)
	matcherFile := filepath.Join(app, "web", "src", "params.go")
	original, err := os.ReadFile(matcherFile)
	if err != nil {
		t.Fatal(err)
	}
	original = bytes.Replace(original, []byte("import ("), []byte(`import (
 domain "github.com/tylergannon/skgo/example/domain"
 other "github.com/tylergannon/skgo/example/other"`), 1)
	original = append(original, []byte(`
func Numeric(s string) (domain.Number,bool) { n,e:=strconv.Atoi(s);return domain.Number(n),e==nil }
func Aliased(s string) (domain.NumberAlias,bool) { return Numeric(s) }
func Other(s string) (other.Number,bool) { return 42,true }
func Flag(s string) (bool,bool) { return false,true }
func Blank(s string) (string,bool) { return "",true }
func MaybeRef(s string) (domain.Ref,bool) { if s=="none" {return nil,true};if s=="42"{return &domain.OrderRef{Number:42},true};return nil,false }
func Pointer(s string) (*domain.OrderRef,bool) { return nil,true }
func Generic(s string) (domain.Box[domain.Number],bool) { return domain.Box[domain.Number]{Value:42},true }
func Structure(s string) (struct{ Value int `+"`json:\"value\"`"+` },bool) { return struct{ Value int `+"`json:\"value\"`"+` }{42},true }
func Function(s string) (func(number int) string,bool) { return func(n int)string{return fmt.Sprint(n)},true }
func FunctionAlias(s string) (func(value int) string,bool) { return Function(s) }
func Slice(s string) ([]domain.Number,bool) { return nil,true }
func Map(s string) (map[string]domain.Number,bool) { return nil,true }
func Channel(s string) (<-chan domain.Number,bool) { return nil,true }
func Interface(s string) (interface{Label() string},bool) { return nil,true }
func Array(s string) ([2]domain.Number,bool) { return [2]domain.Number{0,42},true }
func StructOrder(s string) (domain.Order,bool) { return domain.Order{Number:42},true }
func PublicHidden(s string) (domain.PublicHidden,bool) { return 0,true }
`)...)
	if err := os.WriteFile(matcherFile, original, 0644); err != nil {
		t.Fatal(err)
	}
	frontend := filepath.Join(app, "web", "src", "params.ts")
	frontendSource, err := os.ReadFile(frontend)
	if err != nil {
		t.Fatal(err)
	}
	var entries strings.Builder
	for _, name := range []string{"Numeric", "Aliased", "Other", "Flag", "Blank", "MaybeRef", "Pointer", "Generic", "Structure", "Function", "FunctionAlias", "Slice", "Map", "Channel", "PublicHidden", "StructOrder", "Interface", "Array"} {
		entries.WriteString(name + ": (value: string) => value,\n")
	}
	frontendSource = bytes.Replace(frontendSource, []byte("defineParams({"), []byte("defineParams({\n"+entries.String()), 1)
	if err := os.WriteFile(frontend, frontendSource, 0644); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"numeric/[id=Numeric]", "alias/[id=Aliased]", "text/[id]", "optional/[[id]]", "absent", "other/[id=Other]", "flag/[flag=Flag]", "empty/[empty=Blank]", "pointer/[[pointer=Pointer]]", "nil/[id=MaybeRef]", "nil-optional/[[id=MaybeRef]]", "generic/[box=Generic]", "structure/[structure=Structure]", "fn/[fn=Function]", "fn-alias/[fn=FunctionAlias]", "slice/[slice=Slice]", "map/[map=Map]", "channel/[channel=Channel]", "hidden-alias/[hidden=PublicHidden]", "named/[order=StructOrder]", "interface/[iface=Interface]", "array/[array=Array]", "_frontend/[underscored]", "keys/[slug]/[params]/[request_event]/[1]/[_]/[-]/[a-b]/[a_b]"} {
		writeSharedFixture(t, app, "web/src/routes/"+route+"/+page.svelte", "<p>fixture</p>\n")
	}
	linked := filepath.Join(t.TempDir(), "[linked]")
	if err := os.MkdirAll(linked, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linked, "+page.svelte"), []byte("<p>linked caller</p>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(linked), filepath.Join(app, "web", "src", "routes", "linked")); err != nil {
		t.Fatal(err)
	}
	return app, Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "internal", "skgo")}
}

func runSharedConsumer(t *testing.T, app, pattern string, wantFailure string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-count=1", "-v", "-run", pattern, "./internal/skgo/params")
	cmd.Dir = app
	output, err := cmd.CombinedOutput()
	if wantFailure != "" {
		if err == nil || !strings.Contains(string(output), wantFailure) {
			t.Fatalf("expected compile failure %q: %v\n%s", wantFailure, err, output)
		}
		return
	}
	if err != nil || strings.Contains(string(output), "SKIP") || !strings.Contains(string(output), "--- PASS:") {
		t.Fatalf("generated consumer did not execute: %v\n%s", err, output)
	}
	t.Logf("compiled consumers:\n%s", output)
}

func TestSharedParamsGeneratedConsumers(t *testing.T) {
	t.Parallel()
	app, cfg := sharedFixture(t)
	// A real application body can import shared params before their first generation.
	writeSharedFixture(t, app, "web/src/routes/todos/consumer.go", `package todos
import "github.com/tylergannon/skgo/example/internal/skgo/params"
func sharedConsumer(p params.Params) params.Key_ID { return p.ID() }
`)
	path := filepath.Join(cfg.Out, "params", sharedParamsFile)
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := Check(cfg); err == nil || !strings.Contains(err.Error(), "missing or stale") {
		t.Fatalf("missing detection: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("read-only check wrote params: %v", err)
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(initial), "type IDParam_DomainNumber_") != 2 {
		t.Fatal("aliases failed to deduplicate or distinct packages merged")
	}
	if strings.Count(string(initial), "type FnParam_Func") != 1 {
		t.Fatal("function argument names changed type identity")
	}
	// These names are pinned literals. The running consumer supplies independent
	// concrete values and checks original methods and exact presence receipts.
	writeSharedFixture(t, app, "internal/skgo/params/consumer_test.go", sharedConsumerSource)
	runSharedConsumer(t, app, "^TestSharedReceipts$", "")
	// Plain values must fail interface assignment, not just pass a scanner check.
	for _, value := range []string{"42", `"42"`, "domain.Number(42)"} {
		writeSharedFixture(t, app, "internal/skgo/params/bad_test.go", `package params_test
import ( "github.com/tylergannon/skgo/example/domain"; "github.com/tylergannon/skgo/example/internal/skgo/params" )
var _ = domain.Number(0)
var _ params.Key_ID = `+value+"\n")
		runSharedConsumer(t, app, "^TestSharedReceipts$", "does not implement")
	}
	if err := os.Remove(filepath.Join(cfg.Out, "params", "bad_test.go")); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "web/src/routes/third/[id=Flag]/+page.svelte", "<p>third variant</p>")
	writeSharedFixture(t, app, "web/src/routes/new-key/[slug_string]/+page.svelte", "<p>new key</p>")
	if err := Check(cfg); err == nil || !strings.Contains(err.Error(), "missing or stale") {
		t.Fatalf("stale detection: %v", err)
	}
	afterCheck, _ := os.ReadFile(path)
	if !bytes.Equal(initial, afterCheck) {
		t.Fatal("read-only check mutated stale params")
	}
	// Current declarations must precede checking; failed generation publishes nothing.
	writeSharedFixture(t, app, "web/src/routes/todos/consumer.go", `package todos
import "github.com/tylergannon/skgo/example/internal/skgo/params"
func stale(p params.Params) { p.NoLongerExists() }
`)
	if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "NoLongerExists") {
		t.Fatalf("stale authored body: %v", err)
	}
	afterFailure, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(initial, afterFailure) {
		t.Fatal("failed generation published shared params")
	}
	writeSharedFixture(t, app, "web/src/routes/todos/consumer.go", "package todos\n")
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	refreshed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Slug()", "Params()", "RequestEvent_K726571756573745f6576656e74()", "Key1_K31()", "Key_K5f()", "Key_K2d()", "AB_K612d62()", "AB_K615f62()", "SlugString_K736c75675f737472696e67()", "IDParam_Bool", "Linked()", "Underscored()"} {
		if !strings.Contains(string(refreshed), name) {
			t.Fatalf("stable public declaration %s missing", name)
		}
	}
	for _, line := range strings.Split(string(initial), "\n") {
		if strings.HasPrefix(line, "type ") && strings.Contains(line, "Param_") && !strings.Contains(string(refreshed), line) {
			t.Fatalf("renamed old variant %s", line)
		}
	}
	writeSharedFixture(t, app, "internal/skgo/params/third_test.go", `package params_test
import ("testing"; "github.com/tylergannon/skgo/example/internal/skgo/params")
func TestThird(t *testing.T) {
 p,err:=params.SkgoParams("/third/[id=Flag]",map[string]any{"id":false});if err!=nil{t.Fatal(err)}
 if got:=receipt(p);got!="unsupported"{t.Fatalf("old consumer treats new variant as %s",got)}
 switch id:=p.ID().(type){case params.IDParam_Bool:if id.Value{t.Fatal("false changed")};case nil:t.Fatal("false absent");default:t.Fatalf("wrong variant %T",id)}
}
`)
	runSharedConsumer(t, app, "TestSharedReceipts|TestThird", "")
	final, _ := os.ReadFile(path)
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if !bytes.Equal(final, again) {
		t.Fatal("regeneration unstable")
	}
}

const sharedConsumerSource = `package params_test
import (
 "fmt"
 "reflect"
 "testing"
 "github.com/tylergannon/skgo"
 "github.com/tylergannon/skgo/example/domain"
 other "github.com/tylergannon/skgo/example/other"
 "github.com/tylergannon/skgo/example/internal/skgo/params"
)
var _ skgo.RequestEvent[params.Params] = params.RequestEvent{}
func receipt(p params.Params) string {
 switch id:=p.ID().(type) {
 case params.IDParam_DomainNumber_f2699d13762bca7d8b7b8d63120233e8bd9a519b8afaa1e5fedbb0bc340de94a: return "numeric:"+id.Value.Label()
 case params.IDParam_String: return "string:"+id.Value
 case params.IDParam_Ref: if id.Value==nil{return "remote:present:nil"};return "remote:present:"+id.Value.Label()
 case nil: return "remote:absent"
 }
 return "unsupported"
}
func TestSharedReceipts(t *testing.T) {
 for _,tc:=range []struct{route string; values map[string]any; want string}{
 {"/numeric/[id=Numeric]",map[string]any{"id":domain.Number(42)},"numeric:Order #42"},
 {"/numeric/[id=Numeric]",map[string]any{"id":domain.Number(0)},"numeric:Order #0"},
 {"/alias/[id=Aliased]",map[string]any{"id":domain.NumberAlias(42)},"numeric:Order #42"},
 {"/text/[id]",map[string]any{"id":"42"},"string:42"},
 {"/text/[id]",map[string]any{"id":""},"string:"},
 {"/absent",nil,"remote:absent"},
 {"/optional/[[id]]",nil,"remote:absent"},
 {"/nil/[id=MaybeRef]",map[string]any{"id":nil},"remote:present:nil"},
 {"/nil-optional/[[id=MaybeRef]]",nil,"remote:absent"},
 {"/nil/[id=MaybeRef]",map[string]any{"id":&domain.OrderRef{Number:42}},"remote:present:Ref #42"},
 {"/nil/[id=MaybeRef]",map[string]any{"id":(*domain.OrderRef)(nil)},"typed-nil"},
 } {
  p,err:=params.SkgoParams(tc.route,tc.values);if err!=nil{t.Fatal(err)}
  if tc.want=="typed-nil" {v,ok:=p.ID().(params.IDParam_Ref);if !ok || v.Value==nil || !reflect.ValueOf(v.Value).IsNil(){t.Fatalf("lost declared interface typed nil: %T",p.ID())};continue}
  if got:=receipt(p);got!=tc.want{t.Fatalf("%s receipt %s, want %s",tc.route,got,tc.want)}
  if p.ID()!=nil && reflect.TypeOf(p.ID()).Kind()!=reflect.Struct{t.Fatalf("presence wrapper %T is not concrete",p.ID())}
 }
 named,err:=params.SkgoParams("/named/[order=StructOrder]",map[string]any{"order":domain.Order{Number:42}});if err!=nil{t.Fatal(err)}
 switch v:=named.Order().(type){case params.OrderParam_Order:if v.Value.Number!=42 || v.Value.Label()!="Struct order #42"{t.Fatal("lost named struct fields/method")};default:t.Fatalf("wrong struct variant %T",v)}
 p,err:=params.SkgoParams("/pointer/[[pointer=Pointer]]",map[string]any{"pointer":(*domain.OrderRef)(nil)});if err!=nil{t.Fatal(err)}
 switch v:=p.Pointer().(type){case params.PointerParam_PointerOrderRef:if v.Value!=nil{t.Fatal("pointer payload changed")};case nil:t.Fatal("pointer:absent");default:t.Fatalf("wrong pointer variant %T",v)}
 p,err=params.SkgoParams("/pointer/[[pointer=Pointer]]",nil);if err!=nil || p.Pointer()!=nil{t.Fatal("pointer omission must be absent")}
 p,err=params.SkgoParams("/flag/[flag=Flag]",map[string]any{"flag":false});if err!=nil{t.Fatal(err)}
 switch v:=p.Flag().(type){case params.FlagParam_Bool:if v.Value{t.Fatal("false changed")};default:t.Fatalf("false lost: %T",v)}
 p,err=params.SkgoParams("/empty/[empty=Blank]",map[string]any{"empty":""});if err!=nil{t.Fatal(err)}
 switch v:=p.Empty().(type){case params.EmptyParam_String:if v.Value!=""{t.Fatal("empty value changed")};default:t.Fatalf("empty lost: %T",v)}
 p,err=params.SkgoParams("/other/[id=Other]",map[string]any{"id":other.Number(42)});if err!=nil{t.Fatal(err)}
 if reflect.TypeOf(p.ID()).Field(0).Type!=reflect.TypeFor[other.Number](){t.Fatal("distinct named type lost")}
 for _,tc:=range []struct{route,key string; value any; typ reflect.Type}{
 {"/interface/[iface=Interface]","iface",nil,reflect.TypeFor[interface{Label()string}]()},
 {"/array/[array=Array]","array",[2]domain.Number{0,42},reflect.TypeFor[[2]domain.Number]()},
 {"/generic/[box=Generic]","box",domain.Box[domain.Number]{Value:42},reflect.TypeFor[domain.Box[domain.Number]]()},
 {"/structure/[structure=Structure]","structure",struct{Value int ` + "`json:\"value\"`" + `}{42},reflect.TypeFor[struct{Value int ` + "`json:\"value\"`" + `}]()},
 {"/fn/[fn=Function]","fn",func(n int)string{return fmt.Sprint(n)},reflect.TypeFor[func(int)string]()},
 {"/slice/[slice=Slice]","slice",[]domain.Number(nil),reflect.TypeFor[[]domain.Number]()},
 {"/map/[map=Map]","map",map[string]domain.Number(nil),reflect.TypeFor[map[string]domain.Number]()},
 {"/channel/[channel=Channel]","channel",(<-chan domain.Number)(nil),reflect.TypeFor[<-chan domain.Number]()},
 } {
  p,err:=params.SkgoParams(tc.route,map[string]any{tc.key:tc.value});if err!=nil{t.Fatal(err)}
  getter:=reflect.ValueOf(p).MethodByName(map[string]string{"iface":"Iface","array":"Array","box":"Box","structure":"Structure","fn":"Fn","slice":"Slice","map":"Map","channel":"Channel"}[tc.key])
  v:=getter.Call(nil)[0].Elem();if v.Kind()!=reflect.Struct || v.Field(0).Type()!=tc.typ{t.Fatalf("%s erased declared type",tc.route)}
  if !reflect.DeepEqual(v.Field(0).Interface(),tc.value) && tc.key!="fn"{t.Fatalf("%s changed payload",tc.route)}
  if tc.key=="fn" && v.Field(0).Interface().(func(int)string)(42)!="42"{t.Fatal("function value lost")}
 }
 for _,tc:=range []struct{route string; raw any}{ {"/numeric/[id=Numeric]",nil},{"/text/[id]",nil},{"/nil/[id=MaybeRef]",42} } {
  if _,err:=params.SkgoParams(tc.route,map[string]any{"id":tc.raw});err==nil{t.Fatalf("mismatched nil/type accepted for %s",tc.route)}
 }
}
`

func TestSharedParamsGeneratedNilLoads(t *testing.T) {
	t.Parallel()
	app, cfg := sharedFixture(t)
	for _, tc := range []struct{ route, body string }{
		{"nil/[id=MaybeRef]", `ref:=event.Params.ID();label:="load:present:nil";if ref!=nil{label="load:present:"+ref.Label()}`},
		{"nil-optional/[[id=MaybeRef]]", `ptr:=event.Params.ID();label:="load:absent";if ptr!=nil{label="load:present:nil";if *ptr!=nil{label="load:present:"+(*ptr).Label()}}`},
	} {
		writeSharedFixture(t, app, "web/src/routes/"+tc.route+"/page.server.go", `package nilfixture
import "github.com/tylergannon/skgo"
type PageData struct { Receipt string `+"`json:\"receipt\"`"+` }
func load(event RequestEvent) (PageData,error) { `+tc.body+`;return PageData{Receipt:label},nil }
var _ = skgo.Load(load)
`)
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "internal/skgo/nil_loads_test.go", `package skgo_test
import (
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/tylergannon/skgo"
 generated "github.com/tylergannon/skgo/example/internal/skgo"
)
func TestGeneratedNilLoads(t *testing.T) {
 const required="src/routes/nil/[id=MaybeRef]/+page.server.ts"
 const optional="src/routes/nil-optional/[[id=MaybeRef]]/+page.server.ts"
 var loads []*skgo.ServerLoad
 for _,load:=range generated.Loads(){if load.Module()==required || load.Module()==optional{loads=append(loads,load)}}
 if len(loads)!=2{t.Fatalf("want two generated loads, got %d",len(loads))}
 cfg:=skgo.LoadConfig{Origin:"http://127.0.0.1:8080",Nodes:[]string{required,optional},Routes:[]skgo.ManifestRoute{
 {ID:"/nil/[id=MaybeRef]",Pattern:"^/nil/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"id",Matcher:"MaybeRef"}},Page:&skgo.ManifestPage{Leaf:0}},
 {ID:"/nil-optional/[[id=MaybeRef]]",Pattern:"^/nil-optional(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"id",Matcher:"MaybeRef",Optional:true,Chained:true}},Page:&skgo.ManifestPage{Leaf:1}},
 }}
 handler,err:=skgo.NewLoads(cfg,loads...);if err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{path,want string}{
 {"/nil/none/__data.json","load:present:nil"},
 {"/nil-optional/none/__data.json","load:present:nil"},
 {"/nil-optional/__data.json","load:absent"},
 {"/nil/42/__data.json","load:present:Ref #42"},
 }{
  response:=httptest.NewRecorder();handler.ServeHTTP(response,httptest.NewRequest("GET","http://127.0.0.1:8080"+tc.path,nil))
  if response.Code!=200 || !strings.Contains(response.Body.String(),`+"`"+`{"receipt":1},"`+"`"+`+tc.want+`+"`"+`"`+"`"+`) || !strings.Contains(response.Body.String(),`+"`"+`"params":["id"]`+"`"+`){t.Fatalf("%s want %s and tracked id; %d %s",tc.path,tc.want,response.Code,response.Body.String())}
 }
}
`)
	cmd := exec.Command("go", "test", "-count=1", "-v", "-run", "^TestGeneratedNilLoads$", "./internal/skgo")
	cmd.Dir = app
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "--- PASS: TestGeneratedNilLoads") || strings.Contains(string(output), "SKIP") {
		t.Fatalf("generated nil loads did not run: %v\n%s", err, output)
	}
	t.Logf("real generated nil loads:\n%s", output)
}

func TestSharedParamsSourceDiagnostics(t *testing.T) {
	app, cfg := sharedFixture(t)
	if err := os.RemoveAll(filepath.Join(cfg.Out, "params")); err != nil {
		t.Fatal(err)
	}
	matcherPath := filepath.Join(app, "web", "src", "params.go")
	original, err := os.ReadFile(matcherPath)
	if err != nil {
		t.Fatal(err)
	}
	frontendPath := filepath.Join(app, "web", "src", "params.ts")
	frontend, err := os.ReadFile(frontendPath)
	if err != nil {
		t.Fatal(err)
	}
	frontend = bytes.Replace(frontend, []byte("defineParams({"), []byte("defineParams({ Bad: (value:string) => value,"), 1)
	if err := os.WriteFile(frontendPath, frontend, 0644); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "web/src/routes/diagnostic/[bad=Bad]/+page.svelte", "<p>diagnostic</p>")
	for _, tc := range []struct{ name, importPath, setup, signature, want string }{
		{"private-type", "", "", `type hidden int; func Bad(s string) (hidden,bool) { return 0,true }`, "inaccessible type hidden"},
		{"private-argument", "", "", `type hidden int; func Bad(s string) (domain.Box[hidden],bool) { return domain.Box[hidden]{},true }`, "inaccessible type hidden"},
		{"private-struct-field", "", "", `func Bad(s string) (struct{ private int },bool) { return struct{ private int }{},true }`, "inaccessible structural field private"},
		{"private-interface-method", "", "", `func Bad(s string) (interface{ private() },bool) { return nil,true }`, "inaccessible structural interface method private"},
		{"illegal-internal", "github.com/tylergannon/skgo/example/foreign/internal/domain", `package domain; type Value int`, `func Bad(s string) (bad.Value,bool) { return 0,true }`, "use of internal package"},
		{"remote-owned", "github.com/tylergannon/skgo/example/web/src/lib/remote", `package remote; type Value int`, `func Bad(s string) (bad.Value,bool) { return 0,true }`, "caller-owned"},
		{"caller-owned", "github.com/tylergannon/skgo/example/web/src/routes/owned", `package owned; type Value int`, `func Bad(s string) (bad.Value,bool) { return 0,true }`, "caller-owned"},
		{"generated-package", "github.com/tylergannon/skgo/example/internal/skgo/params", `package params; type Value int`, `func Bad(s string) (bad.Value,bool) { return 0,true }`, "generated package"},
		{"transitive-generated", "github.com/tylergannon/skgo/example/cycledomain", `package cycledomain; import "github.com/tylergannon/skgo/example/internal/skgo/params"; type Value int; var _ params.Value`, `func Bad(s string) (bad.Value,bool) { return 0,true }`, "imports generated bindings/params"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := append([]byte(nil), original...)
			if tc.importPath != "" {
				source = bytes.Replace(source, []byte("import ("), []byte("import ( bad "+`"`+tc.importPath+`"`), 1)
				rel := strings.TrimPrefix(tc.importPath, "github.com/tylergannon/skgo/example/")
				writeSharedFixture(t, app, rel+"/value.go", tc.setup)
				if tc.name == "remote-owned" {
					writeSharedFixture(t, app, rel+"/value.remote.go", "package remote\n")
				}
			}
			source = append(source, []byte("\n"+tc.signature+"\n")...)
			if err := os.WriteFile(matcherPath, source, 0644); err != nil {
				t.Fatal(err)
			}
			files, err := findSourceFiles(cfg.Web)
			if err != nil {
				t.Fatal(err)
			}
			_, err = prepareLoadParams(&cfg, files)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "params.go:") {
				t.Fatalf("want source-located %q: %v", tc.want, err)
			}
		})
	}
	// A real import cycle is diagnosed by the Go compiler with the matcher
	// source location and the domain ownership remedy, rather than bootstrapped
	// using stale generated types.
	writeSharedFixture(t, app, "internal/skgo/params/value.go", `package params; import "github.com/tylergannon/skgo/example/cycledomain"; type Value = cycledomain.Value`)
	source := bytes.Replace(original, []byte("import ("), []byte(`import ( bad "github.com/tylergannon/skgo/example/cycledomain"`), 1)
	source = append(source, []byte(`func Bad(s string) (bad.Value,bool) {return 0,true}`)...)
	if err := os.WriteFile(matcherPath, source, 0644); err != nil {
		t.Fatal(err)
	}
	_, err = prepareLoadParams(&cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "import cycle") || !strings.Contains(err.Error(), "params.go") {
		t.Fatalf("want source-located cycle diagnostic: %v", err)
	}
}

func TestSharedParamsFrontendOnlyApplication(t *testing.T) {
	t.Parallel()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	web := filepath.Join(app, "web")
	out := filepath.Join(app, "internal", "skgo")
	if err := os.RemoveAll(filepath.Join(web, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "web/src/routes/plain/[id]/+page.svelte", "<p>frontend-only</p>")
	writeSharedFixture(t, app, "web/src/routes/optional/[[id]]/+page.svelte", "<p>optional frontend-only</p>")
	cfg := Config{Web: web, Out: out}
	if err := Check(cfg); err == nil || !strings.Contains(err.Error(), "missing or stale") {
		t.Fatalf("hookless/loadless missing output check: %v", err)
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if err := Check(cfg); err != nil {
		t.Fatalf("hookless/loadless read-only check: %v", err)
	}
	writeSharedFixture(t, app, "internal/skgo/params/consumer_test.go", `package params_test
import ("testing"; "github.com/tylergannon/skgo/example/internal/skgo/params")
func TestFrontendOnly(t *testing.T) {
 p,err:=params.SkgoParams("/plain/[id]",map[string]any{"id":"frontend"});if err!=nil{t.Fatal(err)}
 switch v:=p.ID().(type){case params.IDParam_String:if v.Value!="frontend"{t.Fatalf("wrong value %s",v.Value)};default:t.Fatalf("wrong variant %T",v)}
 p,err=params.SkgoParams("/optional/[[id]]",nil);if err!=nil || p.ID()!=nil{t.Fatal("omitted frontend param should be absent")}
}
`)
	runSharedConsumer(t, app, "^TestFrontendOnly$", "")
}
