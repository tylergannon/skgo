package gen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypedLoadJavaScriptMatcherContract(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the independently authored JS entry through installed Kit's
	// matcher execution. Literal JSON also distinguishes numbers from strings.
	command := exec.Command("node", "--input-type=module", "--eval", `
import { parse_route_id, exec } from './node_modules/@sveltejs/kit/src/utils/routing.js';
import { params } from './src/params.ts';
const route = parse_route_id('/typed-load/[number=Order]');
const answers = ['00042','0','+0','1000000','1000001','-1','no','9223372036854775808'].map(value =>
 exec(route.pattern.exec('/typed-load/' + value), route.params, params) ?? null);
console.log(JSON.stringify(answers));
`)
	command.Dir = filepath.Join(root, "example", "web")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Kit JS matcher execution: %v\n%s", err, output)
	}
	const want = `[{"number":42},{"number":0},{"number":0},{"number":1000000},null,null,null,null]`
	if strings.TrimSpace(string(output)) != want {
		t.Fatalf("JS matcher contract: %s, want %s", output, want)
	}
}

func TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation(t *testing.T) {
	t.Parallel()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	if err := stageTypedLoadEvolutionFixture(root, app); err != nil {
		t.Fatal(err)
	}
	cfg := fixtureConfig(Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "internal", "skgo")})
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	paramsFile := filepath.Join(app, "web", "src", "routes", "typed-load", "[number=Order]", loadParamsFile)
	dependenciesFile := filepath.Join(app, "web", "src", "routes", "typed-dependencies", "[a=Order]", "[b=Order]", "[ignored]", "page.server.go")
	paramDeclarations := []struct {
		path      string
		accessors []string
	}{
		{paramsFile, []string{"Number"}},
		{filepath.Join(filepath.Dir(dependenciesFile), loadParamsFile), []string{"A", "B"}},
	}
	initialParams := map[string][]byte{}
	for _, fixture := range paramDeclarations {
		initial, err := os.ReadFile(fixture.path)
		if err != nil {
			t.Fatal(err)
		}
		initialParams[fixture.path] = initial
		declarations := strings.Join(strings.Fields(string(initial)), " ")
		for _, accessor := range fixture.accessors {
			if !strings.Contains(declarations, accessor+"() hooks.OrderNumber") || !strings.Contains(declarations, "value"+accessor+" hooks.OrderNumber") {
				t.Fatalf("%s: initial named matcher type missing: %s", fixture.path, initial)
			}
		}
	}
	matcherFile := filepath.Join(app, "web", "src", "params.go")
	revised := `package hooks
import ("fmt"; "strconv")
type RevisedOrder struct { Number int64 }
func (n RevisedOrder) Text() string { return fmt.Sprintf("Revised order #%d", n.Number) }
func Order(value string) (RevisedOrder, bool) {
 n, err := strconv.ParseInt(value,10,64)
 return RevisedOrder{Number:n}, err == nil && n >= 0 && n <= 1000000
}

`
	if err := os.WriteFile(matcherFile, []byte(revised), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "Label") {
		t.Fatalf("stale handler must fail compilation after params refresh: %v", err)
	}
	for path, initial := range initialParams {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, initial) {
			t.Fatalf("failed generation changed %s", path)
		}
	}

	handlerFile := filepath.Join(app, "web", "src", "routes", "typed-load", "[number=Order]", "page.server.go")
	if err := replaceOnce(handlerFile, "event.Params.Number().Label()", "event.Params.Number().Text()"); err != nil {
		t.Fatal(err)
	}
	for _, param := range []string{"A", "B"} {
		content, err := os.ReadFile(dependenciesFile)
		if err != nil {
			t.Fatal(err)
		}
		before := "event.Params." + param + "().Label()"
		if !strings.Contains(string(content), before) {
			t.Fatalf("missing stale fixture call %s", before)
		}
		if err := os.WriteFile(dependenciesFile, []byte(strings.ReplaceAll(string(content), before, "event.Params."+param+"().Text()")), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(cfg); err != nil {
		t.Fatalf("regeneration after handler repair: %v", err)
	}
	// The repaired code must both compile and answer a real data request with
	// a literal value independent of generation's output.
	consumer := `package skgo_test
import (
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/tylergannon/skgo"
 generated "github.com/tylergannon/skgo/example/internal/skgo"
)
func TestRepairedTypedLoad(t *testing.T) {
 const module = "src/routes/typed-load/[number=Order]/+page.server.ts"
 const dependencies = "src/routes/typed-dependencies/[a=Order]/[b=Order]/[ignored]/+page.server.ts"
 var selected []*skgo.ServerLoad
 for _, load := range generated.Loads() { if load.Module() == module || load.Module() == dependencies { selected=append(selected,load) } }
 if len(selected)!=2 { t.Fatalf("got %d repaired loads, want 2",len(selected)) }
 cfg := skgo.LoadConfig{Origin:"http://127.0.0.1:8080", Nodes:[]string{module,dependencies}, Routes:[]skgo.ManifestRoute{
  {ID:"/typed-load/[number=Order]",Pattern:"^/typed-load/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"number",Matcher:"Order"}},Page:&skgo.ManifestPage{Leaf:0}},
  {ID:"/typed-dependencies/[a=Order]/[b=Order]/[ignored]",Pattern:"^/typed-dependencies/([^/]+?)/([^/]+?)/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"a",Matcher:"Order"},{Name:"b",Matcher:"Order"},{Name:"ignored"}},Page:&skgo.ManifestPage{Leaf:1}},
 }}
 handler, err := skgo.NewLoads(cfg,selected...)
 if err != nil { t.Fatal(err) }
 for _,fixture:=range []struct{path,label string}{
  {"/typed-load/42/__data.json","Revised order #42"},
  {"/typed-dependencies/7/42/first/__data.json","Revised order #7"},
  {"/typed-dependencies/7/42/first/__data.json?read=b","Revised order #42"},
 } {
  response := httptest.NewRecorder()
  handler.ServeHTTP(response,httptest.NewRequest("GET","http://127.0.0.1:8080"+fixture.path,nil))
  if response.Code != 200 || !strings.Contains(response.Body.String(), fixture.label) { t.Fatalf("%s: status %d body %s",fixture.path,response.Code,response.Body.String()) }
 }
}
`
	if err := os.WriteFile(filepath.Join(app, "internal", "skgo", "typed_load_repaired_test.go"), []byte(consumer), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-count=1", "-run", "^TestRepairedTypedLoad$", "./internal/skgo")
	cmd.Dir = app
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("repaired load compile/behavior: %v\n%s", err, output)
	}
}

func TestTypedLoadMatchersPrerenderNamedGoValues(t *testing.T) {
	t.Parallel()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, filepath.Join(t.TempDir(), "example"))
	if err != nil {
		t.Fatal(err)
	}
	web := filepath.Join(app, "web")
	dependencies := filepath.Join(web, "node_modules")
	if err := os.RemoveAll(dependencies); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "example", "web", "node_modules"), dependencies); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(web, "src", "routes", "typed-load", "[number=Order]", "+page.ts")
	if err := os.WriteFile(page, []byte("export const prerender = true;\nexport const entries = () => [{ number: '0' }, { number: '00042' }];\n"), 0644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(dependencies, ".bin", "vp"), "build")
	command.Dir = web
	command.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build typed prerender fixture: %v\n%s", err, output)
	}
	for path, label := range map[string]string{"0": "Order #0", "00042": "Order #42"} {
		artifact, err := os.ReadFile(filepath.Join(web, "build", "prerendered", "typed-load", path+".html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(artifact), label) {
			t.Fatalf("prerender %s lacks %s", path, label)
		}
	}
}

// Generate from real route files, compile the resulting application and ask
// the actual served handler. Neither expected values nor dependency sets come
// from the generated source or from a previous response.
func TestTypedLoadActualParameterDependencies(t *testing.T) {
	t.Parallel()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	optional := filepath.Join(app, "web", "src", "routes", "typed-optional", "[[n=Order]]")
	if err := os.MkdirAll(optional, 0755); err != nil {
		t.Fatal(err)
	}
	const optionalLoad = `package optional
import "github.com/tylergannon/skgo"
type PageData struct { Label string }
func load(event PageRequestEvent) (PageData,error) {
 read,_ := event.SearchParam("read")
 if read == "none" { return PageData{Label:"Constructed only"},nil }
 var value string
 readValue := func() { n:=event.Params.N(); value="Absent"; if n!=nil { value=n.Label() } }
 if read == "untrack" { event.Untrack(readValue) } else { readValue() }
 return PageData{Label:value},nil
}
var _ = skgo.Load(load)
`
	if err := os.WriteFile(filepath.Join(optional, "page.server.go"), []byte(optionalLoad), 0644); err != nil {
		t.Fatal(err)
	}
	const layoutLoad = `package typeddependencies
import "github.com/tylergannon/skgo"
type LayoutData struct { Label string }
func layoutLoad(event LayoutRequestEvent) (LayoutData,error) {
 read,_ := event.SearchParam("layout")
 label := "Layout constructed only"
 if read != "none" {
  readValue := func() { label = "Layout " + event.Params.B().Label() }
  if read == "untrack" { event.Untrack(readValue) } else { readValue() }
 }
 return LayoutData{Label:label},nil
}
var _ = skgo.Load(layoutLoad)
`
	layoutFile := filepath.Join(app, "web", "src", "routes", "typed-dependencies", "[a=Order]", "[b=Order]", "[ignored]", "layout.server.go")
	if err := os.WriteFile(layoutFile, []byte(layoutLoad), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Run(fixtureConfig(Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "internal", "skgo")})); err != nil {
		t.Fatal(err)
	}
	const consumer = `package skgo_test
import (
 "encoding/json"
 "net/http/httptest"
 "reflect"
 "strings"
 "strconv"
 "testing"
 "github.com/tylergannon/skgo"
 generated "github.com/tylergannon/skgo/example/internal/skgo"
 hooks "github.com/tylergannon/skgo/example/web/src"
 params "PARAMS_PACKAGE"
 optionalParams "OPTIONAL_PARAMS_PACKAGE"
)
func TestGeneratedParameterDependencies(t *testing.T) {
 // The route params must actually store concrete values, rather than defer
 // assertions to their accessors. Reflect literal types independently of names.
 for _,tc:=range []struct{value any; want map[reflect.Type]int}{
  {params.RouteParams{},map[reflect.Type]int{reflect.TypeFor[*skgo.Event]():1,reflect.TypeFor[hooks.OrderNumber]():2,reflect.TypeFor[string]():1}},
  {optionalParams.RouteParams{},map[reflect.Type]int{reflect.TypeFor[*skgo.Event]():1,reflect.TypeFor[*hooks.OrderNumber]():1}},
 } {
  typ:=reflect.TypeOf(tc.value)
  got:=map[reflect.Type]int{}
  if typ.Kind()!=reflect.Struct { t.Fatalf("params type is %s, want concrete struct",typ) }
  for i:=0; i<typ.NumField(); i++ {
   field:=typ.Field(i)
   if field.IsExported() { t.Fatalf("exported params storage %s",field.Name) }
   got[field.Type]++
  }
  if !reflect.DeepEqual(got,tc.want) { t.Fatalf("stored param types %v, want %v",got,tc.want) }
 }
 const module="src/routes/typed-dependencies/[a=Order]/[b=Order]/[ignored]/+page.server.ts"
 const layout="src/routes/typed-dependencies/[a=Order]/[b=Order]/[ignored]/+layout.server.ts"
 const optional="src/routes/typed-optional/[[n=Order]]/+page.server.ts"
 var loads []*skgo.ServerLoad
 for _,load:=range generated.Loads() { switch load.Module() { case module,layout,optional: loads=append(loads,load) } }
 if len(loads)!=3 { t.Fatalf("got %d loads, want 3",len(loads)) }
 cfg:=skgo.LoadConfig{Origin:"http://127.0.0.1:8080", Nodes:[]string{layout,module,optional}, Routes:[]skgo.ManifestRoute{
  {ID:"/typed-dependencies/[a=Order]/[b=Order]/[ignored]",Pattern:"^/typed-dependencies/([^/]+?)/([^/]+?)/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"a",Matcher:"Order"},{Name:"b",Matcher:"Order"},{Name:"ignored"}},Page:&skgo.ManifestPage{Layouts:[]int{0},Leaf:1}},
  {ID:"/typed-optional/[[n=Order]]",Pattern:"^/typed-optional(?:/([^/]+))?/?$",Params:[]skgo.ManifestParam{{Name:"n",Matcher:"Order",Optional:true,Chained:true}},Page:&skgo.ManifestPage{Leaf:2}},
 }}
 handler,err:=skgo.NewLoads(cfg,loads...); if err!=nil { t.Fatal(err) }
 type expectedNode struct {label string; params []string}
 cases:=[]struct {path string; nodes []expectedNode}{
  {"/typed-dependencies/0/42/first/__data.json?read=none&layout=none",[]expectedNode{{"Layout constructed only",nil},{"No parameter read",nil}}},
  {"/typed-dependencies/0/42/first/__data.json",[]expectedNode{{"Layout Order #42",[]string{"b"}},{"Order #0",[]string{"a"}}}},
  {"/typed-dependencies/0/42/first/__data.json?read=b&layout=none",[]expectedNode{{"Layout constructed only",nil},{"Order #42",[]string{"b"}}}},
  {"/typed-dependencies/7/42/second/__data.json?layout=untrack",[]expectedNode{{"Layout Order #42",nil},{"Order #7",[]string{"a"}}}},
  {"/typed-dependencies/7/42/second/__data.json?read=nested",[]expectedNode{{"Layout Order #42",[]string{"b"}},{"Order #7",[]string{"a"}}}},
  {"/typed-optional/__data.json?read=none",[]expectedNode{{"Constructed only",nil}}},
  {"/typed-optional/__data.json",[]expectedNode{{"Absent",[]string{"n"}}}},
  {"/typed-optional/0/__data.json",[]expectedNode{{"Order #0",[]string{"n"}}}},
  {"/typed-optional/42/__data.json?read=untrack",[]expectedNode{{"Order #42",nil}}},
 }
 for _,tc:=range cases { t.Run(tc.path,func(t *testing.T){
  rec:=httptest.NewRecorder(); handler.ServeHTTP(rec,httptest.NewRequest("GET","http://127.0.0.1:8080"+tc.path,nil))
  var wire struct {Type string; Nodes []struct {Type string; Data json.RawMessage; Uses struct {Params []string}}}
  if rec.Code!=200 || json.Unmarshal(rec.Body.Bytes(),&wire)!=nil || wire.Type!="data" || len(wire.Nodes)!=len(tc.nodes) { t.Fatalf("invalid response %d %s",rec.Code,rec.Body.String()) }
  for i,want:=range tc.nodes {
   node:=wire.Nodes[i]
   if node.Type!="data" || !strings.Contains(string(node.Data),strconv.Quote(want.label)) { t.Fatalf("node %d missing literal %q: %s",i,want.label,rec.Body.String()) }
   if !reflect.DeepEqual(node.Uses.Params,want.params) { t.Fatalf("node %d params %v, want %v: %s",i,node.Uses.Params,want.params,rec.Body.String()) }
  }
 }) }
}
`
	const linkPrefix = "github.com/tylergannon/skgo/example/internal/skgo/links/"
	linkedConsumer := strings.NewReplacer(
		"PARAMS_PACKAGE", linkPrefix+encodeLinkName("src/routes/typed-dependencies/[a=Order]/[b=Order]/[ignored]"),
		"OPTIONAL_PARAMS_PACKAGE", linkPrefix+encodeLinkName("src/routes/typed-optional/[[n=Order]]"),
	).Replace(consumer)
	if err := os.WriteFile(filepath.Join(app, "internal", "skgo", "typed_dependencies_test.go"), []byte(linkedConsumer), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-count=1", "-run", "^TestGeneratedParameterDependencies$", "./internal/skgo")
	cmd.Dir = app
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated load dependency contracts: %v\n%s", err, output)
	}
}

func TestAppMatchersWithoutGoLoads(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	web := filepath.Join(app, "web")
	for _, dir := range []string{filepath.Join(web, "src", "routes", "orders", "[n=Order]"), filepath.Join(app, "generated")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"go.mod":            "module matcherfixture\n\ngo 1.27\nrequire github.com/tylergannon/skgo v0.0.0\nreplace github.com/tylergannon/skgo => " + root + "\n",
		"web/src/params.js": "export const params = { Order: value => value === '42' ? 42 : undefined };\n",
		"web/src/params.go": "package params\nfunc Order(value string) (int,bool) { return 42,value==\"42\" }\n",
		"web/src/routes/orders/[n=Order]/+page.svelte": "<p>Order</p>\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(app, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := linkGeneratorKit(root, app); err != nil {
		t.Fatal(err)
	}
	cfg := fixtureConfig(Config{Web: web, Out: filepath.Join(app, "generated")})
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	const consumer = `package generated
import("net/http";"net/http/httptest";"testing";"github.com/tylergannon/skgo")
func TestMatcherOnlyHandler(t *testing.T){
 if len(Loads())!=0{t.Fatal("fixture must have no loads")}
 cfg:=skgo.LoadConfig{Nodes:[]string{""},Matchers:Matchers(),Routes:[]skgo.ManifestRoute{{ID:"/orders/[n=Order]",Pattern:"^/orders/([^/]+?)/?$",Params:[]skgo.ManifestParam{{Name:"n",Matcher:"Order"}},Page:&skgo.ManifestPage{Leaf:0}}}}
 ls,err:=skgo.NewLoads(cfg);if err!=nil{t.Fatal(err)}
 for _,c:=range []struct{path string;status int;body string}{{"42",200,"{\"type\":\"data\",\"nodes\":[null]}\n"},{"banana",404,"Not Found\n"}}{
 r:=httptest.NewRecorder();ls.Intercept(http.NotFoundHandler()).ServeHTTP(r,httptest.NewRequest("GET","/orders/"+c.path+"/__data.json",nil))
 if r.Code!=c.status||r.Body.String()!=c.body{t.Fatalf("%s: %d %s",c.path,r.Code,r.Body.String())}
 }
}`
	if err := os.WriteFile(filepath.Join(app, "generated", "matcher_test.go"), []byte(consumer), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-mod=mod", "-count=1", "./generated")
	cmd.Dir = app
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("matcher-only generated handler: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(web, "src", "params.go")); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func(Config) error{Run, Check} {
		if err := check(cfg); err == nil || !strings.Contains(err.Error(), "matcher \"Order\" requires a Go matcher") {
			t.Fatalf("missing Go matcher: %v", err)
		}
	}
}
