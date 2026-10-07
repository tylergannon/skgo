package gen

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const stage2Layout = `package route
import("fmt";"github.com/tylergannon/skgo")
type LayoutData struct { Layout string }
type LayoutAlias = LayoutRequestEvent
func layout(event LayoutAlias)(LayoutData,error){
 mode:=event.Request().URL.Query().Get("mode")
 label:="constructed"
 read:=func(){
  label=event.Params.Org().Label()+"|"
  switch v:=event.Params.ID().(type){
  case nil:label+="absent"
  case IDParam_Number:label+=v.Value.Label()
  case IDParam_String:label+="text:"+v.Value
  case IDParam_PointerThing:if v.Value!=nil{panic("expected nil pointer")};label+="pointer:nil"
  case IDParam_Ref:if v.Value!=nil{panic("expected nil interface")};label+="interface:nil"
  default:panic(fmt.Sprintf("wrong alternative %T",v))
  }
 }
 switch mode {
 case "none":
 case "route":label=event.RouteID()
 case "untrack":event.Untrack(func(){read();_ = event.RouteID()})
 case "flag":v:=event.Params.Flag();label="flag:absent";if v!=nil {label=fmt.Sprintf("flag:%t",*v)}
 case "blank":v:=event.Params.Blank();label="blank:absent";if v!=nil {label="blank:"+*v}
 case "ptr":v:=event.Params.Ptr();label="ptr:absent";if v!=nil {if *v!=nil {panic("not nil")};label="ptr:present-nil"}
 case "ref":v:=event.Params.Ref();label="ref:absent";if v!=nil {if *v!=nil {panic("not nil")};label="ref:present-nil"}
 default:read()
 }
 return LayoutData{Layout:label},nil
}
var _=skgo.Load(layout)
`

// One real generated app supplies the graph, consumers and handler proof.
// Literal routes and responses below are independent of generated metadata.
func stage2Fixture(t *testing.T) (string, Config) {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	write := func(name, source string) { t.Helper(); writeSharedFixture(t, app, name, source) }
	mod, err := os.ReadFile(filepath.Join(root, "example/go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.mod", strings.Replace(string(mod), "module github.com/tylergannon/skgo/example", "module stage2.example/app", 1))
	if err := rewriteSkgoReplace(filepath.Join(app, "go.mod"), root); err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(root, "example/go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", string(sum))

	write("domain/domain.go", `package domain
import "fmt"
type Org string
func (o Org) Label()string{return "org:"+string(o)}
type Number int
func(n Number) Label()string{return fmt.Sprintf("number:%d",n)}
type NumberAlias=Number
type Thing struct{}
type Ref interface{Label()string}
`)
	write("web/src/params.go", `package matchers
import("strconv";"stage2.example/app/domain")
func Org(s string)(domain.Org,bool){return domain.Org(s),s!=""}
func Number(s string)(domain.Number,bool){n,e:=strconv.Atoi(s);return domain.Number(n),e==nil}
func Alias(s string)(domain.NumberAlias,bool){return Number(s)}
func Flag(s string)(bool,bool){return false,s=="false"}
func Blank(s string)(string,bool){return "",s=="blank"}
func Pointer(s string)(*domain.Thing,bool){return nil,s=="nil"}
func Ref(s string)(domain.Ref,bool){return nil,s=="nil"}
`)
	write("web/src/params.js", "export const params={Org:v=>v,Number:v=>+v,Alias:v=>+v,Flag:v=>false,Blank:v=>'',Pointer:v=>null,Ref:v=>null};\n")
	write("web/src/routes/layout.server.go", `package route
import "github.com/tylergannon/skgo"
type RootData struct { Root string }
func root(e LayoutRequestEvent)(RootData,error){return RootData{Root:"root"},nil}
var _=skgo.Load(root)
`)
	write("web/src/routes/+page.svelte", "<p>root</p>")
	const optional = "web/src/routes/optional/[[flag=Flag]]/"
	for _, kind := range []string{"Page", "Layout"} {
		write(optional+strings.ToLower(kind)+".server.go", `package route
import "github.com/tylergannon/skgo"
type `+kind+`Data struct { `+kind+` string }
func `+strings.ToLower(kind)+`(e `+kind+`RequestEvent)(`+kind+`Data,error){
 var value *bool = e.Params.Flag()
 label:="local:absent"
 if value!=nil {if *value {panic("expected false")};label="local:false"}
 return `+kind+`Data{`+kind+`:label},nil
}
var _=skgo.Load(`+strings.ToLower(kind)+`)
`)
	}
	write(optional+"+page.svelte", "<p>optional local</p>")
	write("web/src/routes/(team)/+layout.svelte", "<slot />")
	const org = "web/src/routes/(team)/[org=Org]/"
	write(org+"layout.server.go", stage2Layout)
	write(org+"page.server.go", `package route
import "github.com/tylergannon/skgo"
type PageData struct { Page string }
type PageAlias=PageRequestEvent
func page(e PageAlias)(PageData,error){return PageData{Page:e.Params.Org().Label()},nil}
var _=skgo.Load(page)
`)
	write(org+"+page.svelte", "<p>colocated</p>")
	for _, route := range []string{"number/[id=Number]", "alias/[id=Alias]", "text/[id]", "flag/[[flag=Flag]]", "blank/[[blank=Blank]]", "pointer/[[ptr=Pointer]]", "interface/[[ref=Ref]]", "nil/[id=Pointer]", "iface/[id=Ref]"} {
		write(org+route+"/+page.svelte", "<p>descendant</p>")
	}
	for _, route := range []string{"number/[id=Number]", "alias/[id=Alias]"} {
		write(org+route+"/page.server.go", `package route
import "github.com/tylergannon/skgo"
type PageData struct {Page string}
func page(e PageRequestEvent)(PageData,error){
 label:="page:constructed"
 if e.Request().URL.Query().Get("mode")!="none" && e.Request().URL.Query().Get("mode")!="route" {label=e.Params.ID().Label()}
 return PageData{Page:label},nil
}
var _=skgo.Load(page)
`)
	}
	write(org+"nested/+layout@.svelte", "<slot />")
	write(org+"nested/layout.server.go", `package route
import "github.com/tylergannon/skgo"
type Data struct{Nested string}
func nested(e LayoutRequestEvent)(Data,error){return Data{Nested:e.Params.Org().Label()},nil}
var _=skgo.Load(nested)
`)
	write(org+"nested/[reset]/+page.svelte", "<p>reset layout</p>")
	write(org+"skip/[skipped]/+page@.svelte", "<p>reset page</p>")
	write(org+"named/[chosen]/+page@(team).svelte", "<p>named parent</p>")
	write(org+"isolated/[id=Flag]/+page@.svelte", "<p>isolated matcher</p>")
	write(org+"api/[endpoint]/server.go", `package route
import("net/http";"github.com/tylergannon/skgo")
func endpoint(w http.ResponseWriter,r *http.Request){w.WriteHeader(204)}
var _=skgo.GET(endpoint)
`)
	if err := linkGeneratorKit(root, app); err != nil {
		t.Fatal(err)
	}
	return app, fixtureConfig(Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated")})
}

func stage2GoTest(t *testing.T, app, pattern string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-count=1", "-v", "-run", pattern, "./generated")
	cmd.Dir = app
	output, err := cmd.CombinedOutput()
	if err != nil || strings.Contains(string(output), "--- SKIP:") {
		t.Fatalf("generated consumer did not pass: %v\n%s", err, output)
	}
	for _, name := range []string{"TestAddedRoute", "TestStage2Handler", "TestStage2DomainShapes", "TestStage2Evolution"} {
		if !regexp.MustCompile(pattern).MatchString(name) {
			continue
		}
		if !strings.Contains(string(output), "=== RUN   "+name+"\n") || !strings.Contains(string(output), "--- PASS: "+name+" (") {
			t.Fatalf("selected %s did not run: %s", name, output)
		}
	}
}

func TestStage2LayoutMetadataAcrossLanguageSwitches(t *testing.T) {
	app, cfg := stage2Fixture(t)
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Use the actual fixture the dev browser scenario installs.
	for _, name := range []string{"page.server.go", "+page.svelte"} {
		source, err := os.ReadFile(filepath.Join(root, "example/e2e/fixtures/go-dev-added", name+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeSharedFixture(t, app, "web/src/routes/go-dev-added/"+name, string(source))
	}
	initial := start(func() error { return Run(cfg) })
	t.Parallel()
	if err := initial(); err != nil {
		t.Fatal(err)
	}
	for i, language := range []Language{LanguageTypeScript, LanguageJavaScript, LanguageTypeScript} {
		cfg.Language = language
		// Check must read the pending language without changing stale owned stubs.
		if language == LanguageJavaScript {
			before := checkSourceSnapshot(t, cfg.Web, cfg.Out)
			if err := Check(cfg); err != nil && !strings.Contains(err.Error(), "missing or stale") {
				t.Fatalf("checking pending %s: %v", language, err)
			}
			if after := checkSourceSnapshot(t, cfg.Web, cfg.Out); !reflect.DeepEqual(before, after) {
				t.Fatal("pending-language Check changed source")
			}
		}
		if i > 0 {
			if err := Run(cfg); err != nil {
				t.Fatalf("generating %s: %v", language, err)
			}
		}

		other := ".js"
		if language.JavaScript() {
			other = ".ts"
		}
		for _, module := range []string{"layout", "page"} {
			path := filepath.Join(app, "web/src/routes/(team)/[org=Org]", "+"+module+".server"+other)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("stale owned %s remains: %v", path, err)
			}
		}
		writeSharedFixture(t, app, "generated/added_route_test.go", `package generated
import("net/http/httptest";"strings";"testing";"github.com/tylergannon/skgo")
func TestAddedRoute(t *testing.T){
 cfg:=skgo.LoadConfig{Origin:"http://example.test",Nodes:[]string{"src/routes/go-dev-added/+page.server`+language.ext()+`"},Routes:[]skgo.ManifestRoute{{ID:"/go-dev-added",Pattern:"^/go-dev-added/?$",Page:&skgo.ManifestPage{Leaf:0}}}}
 h,err:=skgo.NewLoads(cfg,Loads()...);if err!=nil{t.Fatal(err)}
 rec:=httptest.NewRecorder();h.ServeHTTP(rec,httptest.NewRequest("GET","http://example.test/go-dev-added/__data.json",nil))
 if rec.Code!=200 || !strings.Contains(rec.Body.String(),"New Go route revision one"){t.Fatalf("added route: %d %s",rec.Code,rec.Body.String())}
}
`)
		stage2GoTest(t, app, "^TestAddedRoute$")
	}
	if err := Check(cfg); err != nil {
		t.Fatalf("checking final TypeScript generation: %v", err)
	}
	// An authored counterpart is a real conflict, never an obsolete SKGo output.
	authored := filepath.Join(app, "web/src/routes/(team)/[org=Org]/+page.server.js")
	if err := os.WriteFile(authored, []byte("export const load = () => ({});\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func(Config) error{Check, Run} {
		if err := check(cfg); err == nil || !strings.Contains(err.Error(), "route_duplicate_files") {
			t.Fatalf("authored counterpart must remain a conflict: %v", err)
		}
	}
	if source, err := os.ReadFile(authored); err != nil || string(source) != "export const load = () => ({});\n" {
		t.Fatalf("authored counterpart changed: %q, %v", source, err)
	}
}

func TestStage2GeneratedLayoutDomainsAndTracking(t *testing.T) {
	t.Parallel()
	app, cfg := stage2Fixture(t)
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if err := Check(cfg); err != nil {
		t.Fatal(err)
	}
	const links = "stage2.example/app/generated/links/"
	consumer := strings.NewReplacer("ORG_PACKAGE", links+encodeLinkName("src/routes/(team)/[org=Org]"), "ROOT_PACKAGE", links+encodeLinkName("src/routes"), "NESTED_PACKAGE", links+encodeLinkName("src/routes/(team)/[org=Org]/nested")).Replace(stage2Consumer)
	writeSharedFixture(t, app, "generated/layout_domains_test.go", consumer)
	// Exact methods do not prove that reset-only matcher alternatives are
	// absent from the sealed union. Inspect that generated package scope too.
	source, err := os.ReadFile(filepath.Join(app, "web/src/routes/(team)/[org=Org]/skgo_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "skgo_gen.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scope.Lookup("IDParam_Bool") != nil {
		t.Fatal("reset matcher widened skipped layout union")
	}
	stage2GoTest(t, app, "^Test(Stage2Handler|Stage2DomainShapes)$")
	// Generation sees the edited participating page set through the same overlay.
	writeSharedFixture(t, app, "web/src/routes/(team)/[org=Org]/later/[later]/+page.svelte", "<p>new descendant</p>")
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "generated/evolution_test.go", `package generated
import("testing";org "`+links+encodeLinkName("src/routes/(team)/[org=Org]")+`")
func TestStage2Evolution(t *testing.T){var _ *string = org.LayoutParams{}.Later()}
`)
	stage2GoTest(t, app, "^TestStage2Evolution$")
}

const stage2Consumer = `package generated
import(
 "encoding/json";"net/http";"net/http/httptest";"reflect";"strings";"testing"
 "github.com/tylergannon/skgo"
 "stage2.example/app/domain"
 org "ORG_PACKAGE"
 root "ROOT_PACKAGE"
 nested "NESTED_PACKAGE"
)
func TestStage2DomainShapes(t *testing.T){
 var _ domain.Org = org.LayoutParams{}.Org()
 var _ domain.Org = org.RouteParams{}.Org()
 var _ *bool = org.LayoutParams{}.Flag()
 var _ *string = org.LayoutParams{}.Blank()
 var _ **domain.Thing = org.LayoutParams{}.Ptr()
 var _ *domain.Ref = org.LayoutParams{}.Ref()
 var _ *string = nested.LayoutParams{}.Reset()
 var _ domain.Org = nested.LayoutParams{}.Org()
 var _ *domain.Org = root.LayoutParams{}.Org()
 // A reset's matcher belongs to root but must not widen the skipped layout.
 var _ root.Key_ID = root.IDParam_Bool{Value:false}
 for _,tc:=range []struct{value any;names []string}{
  {org.LayoutParams{},[]string{"Blank","Flag","ID","Org","Ptr","Ref"}},
  {org.RouteParams{},[]string{"Org"}},
  {nested.LayoutParams{},[]string{"Org","Reset"}},
  {root.LayoutParams{},[]string{"Blank","Chosen","Flag","ID","Org","Ptr","Ref","Reset","Skipped"}},
 }{
  typ:=reflect.TypeOf(tc.value);var names []string
  for i:=0;i<typ.NumMethod();i++{names=append(names,typ.Method(i).Name)}
  if !reflect.DeepEqual(names,tc.names){t.Fatalf("%T methods %v want %v",tc.value,names,tc.names)}
  for i:=0;i<typ.NumField();i++{f:=typ.Field(i);if f.IsExported(){t.Fatalf("exported storage %s",f.Name)}}
 }
 // NumberAlias preserves Go identity and cannot create another alternative.
 var _ org.Key_ID = org.IDParam_Number{Value:domain.NumberAlias(0)}
 if reflect.TypeFor[org.PageRequestEvent]()==reflect.TypeFor[org.LayoutRequestEvent](){t.Fatal("colocated events lost distinct identity")}
}
func TestStage2Handler(t *testing.T){
 const base="src/routes/(team)/[org=Org]/"
	cfg:=skgo.LoadConfig{Origin:"http://example.test",Nodes:[]string{"src/routes/+layout.server.ts",base+"+layout.server.ts",base+"number/[id=Number]/+page.server.ts",base+"alias/[id=Alias]/+page.server.ts",base+"+page.server.ts",base+"nested/+layout.server.ts","","src/routes/optional/[[flag=Flag]]/+layout.server.ts","src/routes/optional/[[flag=Flag]]/+page.server.ts"},Matchers:Matchers(),Routes:[]skgo.ManifestRoute{
	  {ID:"/optional/[[flag=Flag]]",Pattern:"^/optional(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"flag",Matcher:"Flag",Optional:true,Chained:true}},Page:&skgo.ManifestPage{Layouts:[]int{0,7},Leaf:8}},
  {ID:"/",Pattern:"^/$",Page:&skgo.ManifestPage{Layouts:[]int{0},Leaf:6}},
  {ID:"/(team)/[org=Org]",Pattern:"^/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:4}},
  {ID:"/(team)/[org=Org]/number/[id=Number]",Pattern:"^/([^/]+?)/number/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"id",Matcher:"Number"}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:2}},
  {ID:"/(team)/[org=Org]/alias/[id=Alias]",Pattern:"^/([^/]+?)/alias/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"id",Matcher:"Alias"}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:3}},
  {ID:"/(team)/[org=Org]/text/[id]",Pattern:"^/([^/]+?)/text/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"id"}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:6}},
  {ID:"/(team)/[org=Org]/nil/[id=Pointer]",Pattern:"^/([^/]+?)/nil/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"id",Matcher:"Pointer"}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:6}},
  {ID:"/(team)/[org=Org]/iface/[id=Ref]",Pattern:"^/([^/]+?)/iface/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"id",Matcher:"Ref"}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:6}},
  {ID:"/(team)/[org=Org]/flag/[[flag=Flag]]",Pattern:"^/([^/]+?)/flag(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"flag",Matcher:"Flag",Optional:true,Chained:true}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:6}},
  {ID:"/(team)/[org=Org]/blank/[[blank=Blank]]",Pattern:"^/([^/]+?)/blank(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"blank",Matcher:"Blank",Optional:true,Chained:true}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:6}},
  {ID:"/(team)/[org=Org]/pointer/[[ptr=Pointer]]",Pattern:"^/([^/]+?)/pointer(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"ptr",Matcher:"Pointer",Optional:true,Chained:true}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:6}},
  {ID:"/(team)/[org=Org]/interface/[[ref=Ref]]",Pattern:"^/([^/]+?)/interface(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"ref",Matcher:"Ref",Optional:true,Chained:true}},Page:&skgo.ManifestPage{Layouts:[]int{0,1},Leaf:6}},
  {ID:"/(team)/[org=Org]/nested/[reset]",Pattern:"^/([^/]+?)/nested/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"reset"}},Page:&skgo.ManifestPage{Layouts:[]int{0,5},Leaf:6}},
  {ID:"/(team)/[org=Org]/skip/[skipped]",Pattern:"^/([^/]+?)/skip/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"skipped"}},Page:&skgo.ManifestPage{Layouts:[]int{0},Leaf:6}},
  {ID:"/(team)/[org=Org]/named/[chosen]",Pattern:"^/([^/]+?)/named/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"org",Matcher:"Org"},{Name:"chosen"}},Page:&skgo.ManifestPage{Layouts:[]int{0},Leaf:6}},
 }}
 h,err:=skgo.NewLoads(cfg,Loads()...);if err!=nil{t.Fatal(err)}
 // Exercise the real typed boundary and generated load adapters. A separate
 // load must not inherit the reader's dependencies.
 handler:=RequestBoundary(skgo.HandleConfig{Origin:cfg.Origin,Loads:h},h.Intercept(http.NotFoundHandler()))
 type node struct{label string;params []string;route bool}
	 cases:=[]struct{path string;nodes []node}{
	  {"/optional/__data.json",[]node{{"root",nil,false},{"local:absent",[]string{"flag"},false},{"local:absent",[]string{"flag"},false}}},
	  {"/optional/false/__data.json",[]node{{"root",nil,false},{"local:false",[]string{"flag"},false},{"local:false",[]string{"flag"},false}}},
  {"/acme/number/0/__data.json?mode=none",[]node{{"root",nil,false},{"constructed",nil,false},{"page:constructed",nil,false}}},
  {"/acme/number/0/__data.json",[]node{{"root",nil,false},{"org:acme|number:0",[]string{"org","id"},false},{"number:0",[]string{"id"},false}}},
  {"/acme/alias/42/__data.json",[]node{{"root",nil,false},{"org:acme|number:42",[]string{"org","id"},false},{"number:42",[]string{"id"},false}}},
  {"/acme/number/42/__data.json?mode=route",[]node{{"root",nil,false},{"/(team)/[org=Org]/number/[id=Number]",nil,true},{"page:constructed",nil,false}}},
  {"/acme/number/42/__data.json?mode=untrack",[]node{{"root",nil,false},{"org:acme|number:42",nil,false},{"number:42",[]string{"id"},false}}},
  {"/acme/text/word/__data.json",[]node{{"root",nil,false},{"org:acme|text:word",[]string{"org","id"},false},{}}},
  {"/acme/nil/nil/__data.json",[]node{{"root",nil,false},{"org:acme|pointer:nil",[]string{"org","id"},false},{}}},
  {"/acme/iface/nil/__data.json",[]node{{"root",nil,false},{"org:acme|interface:nil",[]string{"org","id"},false},{}}},
  {"/acme/__data.json",[]node{{"root",nil,false},{"org:acme|absent",[]string{"org","id"},false},{"org:acme",[]string{"org"},false}}},
  {"/acme/flag/__data.json?mode=flag",[]node{{"root",nil,false},{"flag:absent",[]string{"flag"},false},{}}},
  {"/acme/flag/false/__data.json?mode=flag",[]node{{"root",nil,false},{"flag:false",[]string{"flag"},false},{}}},
  {"/acme/blank/__data.json?mode=blank",[]node{{"root",nil,false},{"blank:absent",[]string{"blank"},false},{}}},
  {"/acme/blank/blank/__data.json?mode=blank",[]node{{"root",nil,false},{"blank:",[]string{"blank"},false},{}}},
  {"/acme/pointer/__data.json?mode=ptr",[]node{{"root",nil,false},{"ptr:absent",[]string{"ptr"},false},{}}},
  {"/acme/pointer/nil/__data.json?mode=ptr",[]node{{"root",nil,false},{"ptr:present-nil",[]string{"ptr"},false},{}}},
  {"/acme/interface/__data.json?mode=ref",[]node{{"root",nil,false},{"ref:absent",[]string{"ref"},false},{}}},
  {"/acme/interface/nil/__data.json?mode=ref",[]node{{"root",nil,false},{"ref:present-nil",[]string{"ref"},false},{}}},
  {"/acme/nested/value/__data.json",[]node{{"root",nil,false},{"org:acme",[]string{"org"},false},{}}},
  {"/acme/skip/value/__data.json",[]node{{"root",nil,false},{}}},
  {"/acme/named/value/__data.json",[]node{{"root",nil,false},{}}},
  {"/__data.json",[]node{{"root",nil,false},{}}},
 }
 for _,tc:=range cases{t.Run(tc.path,func(t *testing.T){
  rec:=httptest.NewRecorder();handler.ServeHTTP(rec,httptest.NewRequest("GET","http://example.test"+tc.path,nil))
  var wire struct{Type string;Nodes []struct{Type string;Data json.RawMessage;Uses struct{Params []string;Route int;URL int;Parent int;Dependencies []string;SearchParams []string ` + "`json:\"search_params\"`" + `}}}
  if rec.Code!=200 || json.Unmarshal(rec.Body.Bytes(),&wire)!=nil || wire.Type!="data" || len(wire.Nodes)!=len(tc.nodes){t.Fatalf("bad envelope %d %s",rec.Code,rec.Body.String())}
  for i,want:=range tc.nodes{
   got:=wire.Nodes[i]
   if want.label=="" {if got.Type!=""{t.Fatalf("node %d should be null: %s",i,rec.Body.String())};continue}
   if got.Type!="data" || !strings.Contains(string(got.Data),` + "`\"`" + `+want.label+` + "`\"`" + `){t.Fatalf("node %d missing %q: %s",i,want.label,rec.Body.String())}
   if !reflect.DeepEqual(got.Uses.Params,want.params)||(got.Uses.Route!=0)!=want.route||got.Uses.Route<0||got.Uses.Route>1||got.Uses.URL!=0||got.Uses.Parent!=0||len(got.Uses.Dependencies)!=0||len(got.Uses.SearchParams)!=0{t.Fatalf("node %d unexpected uses %+v want params %v route %t",i,got.Uses,want.params,want.route)}
  }
 })}
 // Root construction also works in the fallback/error domain, without a page.
 fallbackHandler:=RequestBoundary(skgo.HandleConfig{},http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  fallback:=root.SkgoLayoutRequestEvent(skgo.EventFrom(r.Context()))
  if fallback.Params.Org()!=nil || fallback.Params.ID()!=nil || fallback.RouteID()!="" {t.Fatal("root fallback must preserve absence and empty route")}
  w.WriteHeader(204)
 }))
 rec:=httptest.NewRecorder();fallbackHandler.ServeHTTP(rec,httptest.NewRequest("GET","/unmatched",nil))
 if rec.Code!=204 {t.Fatalf("fallback event failed: %d %s",rec.Code,rec.Body.String())}
}
`

func TestStage2RejectsInvalidEventAndParameterDomains(t *testing.T) {
	t.Parallel()
	app, cfg := stage2Fixture(t)
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	const route = "web/src/routes/(team)/[org=Org]/"
	originals := map[string]string{}
	for _, file := range []string{"page.server.go", "layout.server.go"} {
		raw, err := os.ReadFile(filepath.Join(app, route, file))
		if err != nil {
			t.Fatal(err)
		}
		originals[file] = string(raw)
	}
	const number = "stage2.example/app/generated/links/"
	cases := []struct{ name, file, source, want string }{
		{"page-receives-layout", "page.server.go", `package route
import "github.com/tylergannon/skgo"
func page(LayoutRequestEvent)(PageData,error){return PageData{},nil}
var _=skgo.Load(page)
type PageData struct{Page string}
`, "generated PageRequestEvent"},
		{"layout-receives-page", "layout.server.go", `package route
import "github.com/tylergannon/skgo"
func layout(PageRequestEvent)(LayoutData,error){return LayoutData{},nil}
var _=skgo.Load(layout)
type LayoutData struct{Layout string}
`, "generated LayoutRequestEvent"},
		{"command-cannot-receive-page", "bad.remote.go", `package route
import("context";"github.com/tylergannon/skgo")
func command(ctx context.Context,e PageRequestEvent)(string,error){return "",nil}
var _=skgo.Command(command)
`, "generated/params.Params"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(app, route, tc.file)
			if err := os.WriteFile(path, []byte(tc.source), 0644); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if original, ok := originals[tc.file]; ok {
					if err := os.WriteFile(path, []byte(original), 0644); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
			}()
			for _, check := range []func(Config) error{Check} {
				err := check(cfg)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("wanted rejection %q, got %v", tc.want, err)
				}
			}
		})
	}
	// The sealed method cannot be implemented outside its declaring package.
	external := strings.NewReplacer("ORG_PACKAGE", number+encodeLinkName("src/routes/(team)/[org=Org]")).Replace(`package generated
import org "ORG_PACKAGE"
type forged struct{}
func(forged) skgoParam_6964(){}
var _ org.Key_ID = forged{}
`)
	writeSharedFixture(t, app, "generated/forged.go", external)
	cmd := exec.Command("go", "test", "-run", "^$", "./generated")
	cmd.Dir = app
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "unexported method skgoParam_6964") {
		t.Fatalf("forged alternative should fail: %v\n%s", err, output)
	}
}
