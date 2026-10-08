package gen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const nativeModelSource = `package data
import (
 "context"
 "math"
 "sync"
 "time"
 "github.com/tylergannon/skgo"
 "github.com/tylergannon/polytype"
 "example.com/app/generated/params"
 "example.com/wire"
 other "example.com/wire/other/wire"
)
type State string
func(State)enum(){}
const(Open State="open";OpenAlias State="open";Closed State="closed")
type Choice interface { choice() }
type Note struct { Text string }
func(Note)choice(){}
type Count struct { Value int64 }
func(*Count)choice(){}
type Node struct { Name string; Next polytype.Nullable[*Node] }
type Envelope struct {
 Text string
 Optional polytype.Optional[string] ` + "`json:\"optional,omitzero\"`" + `
 Nullable polytype.Nullable[string]
 State State
 Values []int64
 Fixed [2]uint8
 Time time.Time
 Choice Choice
 OptionalChoice polytype.Optional[Choice] ` + "`json:\"optionalChoice,omitzero\"`" + `
 Choices []Choice
 Node Node
 First wire.Thing
 Second other.Thing
 Hidden uint64 ` + "`json:\"-\"`" + `
}
func echo(_ context.Context, _ params.RequestEvent, in Envelope)(Envelope,error){ return in,nil }
func read(context.Context)(Envelope,error){ return Envelope{
 Text:"Native voice transcript: café 😀", Nullable:polytype.Nullable[string]{},State:Open,
 Values:[]int64{1,2},Fixed:[2]uint8{7,9},Time:time.Date(2026,10,8,1,2,3,123456789,time.UTC),
 Choice:Note{Text:"hello"},Choices:nil,Node:Node{Name:"leaf"},
 First:wire.Thing{Name:"first",Status:wire.StatusOn},Second:other.Thing{Label:"second"},
 Hidden:18446744073709551615,
 },nil }
var revisionMu sync.Mutex
var revision int64 = 1
func counter(context.Context)(int64,error){revisionMu.Lock();defer revisionMu.Unlock();return revision,nil}
func increment(ctx context.Context,_ params.RequestEvent,delta int64)(int64,error){
 revisionMu.Lock();revision+=delta;value:=revision;revisionMu.Unlock()
 return value,skgo.RefreshRequestedNoArg(ctx,counter)
}
func ignoreCounter(ctx context.Context,_ params.RequestEvent)(string,error){return "ignored",skgo.IgnoreRequestedNoArg(ctx,counter)}
func forgetCounter(context.Context,params.RequestEvent)(string,error){return "forgot",nil}
func wide(_ context.Context)(uint64,error){return 9007199254740993,nil}
var WideCalls int
func wideInput(_ context.Context,_ params.RequestEvent,in uint64)(uint64,error){ WideCalls++;return in,nil }
var SignedCalls, NumberCalls int
func signedInput(_ context.Context,_ params.RequestEvent,in int64)(int64,error){ SignedCalls++;return in,nil }
func numberInput(_ context.Context,_ params.RequestEvent,in float64)(float64,error){ NumberCalls++;return in,nil }
func wideSigned(context.Context)(int64,error){ return -9007199254740993,nil }
func wideNumber(context.Context)(float64,error){ return math.Inf(1),nil }
var(_=skgo.Command(echo);_=skgo.Query(read);_=skgo.Query(wide);_=skgo.Command(wideInput);_=skgo.Command(signedInput);_=skgo.Command(numberInput);_=skgo.Query(wideSigned);_=skgo.Query(wideNumber);_=skgo.Query(counter);_=skgo.Command(increment);_=skgo.Command(ignoreCounter);_=skgo.Command(forgetCounter))
`

var nativeFixture struct {
	sync.Once
	root string
	cfg  Config
	err  error
}

func generatedNativeFixture(t *testing.T) (string, Config) {
	t.Helper()
	nativeFixture.Do(func() {
		nativeFixture.err = fmt.Errorf("native fixture setup did not complete")
		root, cfg := nativeModelFixture(t, filepath.Join(packageTemp, "native"), nativeModelSource)
		cfg.SwiftOut = filepath.Join(root, "app", "native", "Generated.swift")
		cfg.SwiftRemotes = []string{"src/data/data.remote.ts#echo", "src/data/data.remote.ts#read", "src/data/data.remote.ts#wide", "src/data/data.remote.ts#wideInput", "src/data/data.remote.ts#signedInput", "src/data/data.remote.ts#numberInput", "src/data/data.remote.ts#wideSigned", "src/data/data.remote.ts#wideNumber"}
		cfg.SwiftRemotes = append(cfg.SwiftRemotes, "src/data/data.remote.ts#counter", "src/data/data.remote.ts#increment", "src/data/data.remote.ts#ignoreCounter", "src/data/data.remote.ts#forgetCounter")
		nativeFixture.root, nativeFixture.cfg = root, cfg
		nativeFixture.err = Run(cfg)
	})
	if nativeFixture.err != nil {
		t.Fatal(nativeFixture.err)
	}
	return nativeFixture.root, nativeFixture.cfg
}

func nativeModelFixture(t *testing.T, root, source string) (string, Config) {
	t.Helper()
	root, cfg := foreignFixtureIn(t, root, source, map[string]string{
		"app/web/src/data/schema.go": "//go:build jsonschema\npackage data\nimport \"github.com/tylergannon/polytype\"\nvar _=polytype.SealedUnion[Choice](\"kind\")\n",
	})
	repo, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(root, "app", "go.mod")
	data, err := os.ReadFile(mod)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("replace github.com/tylergannon/skgo => ../skgo"), []byte("replace github.com/tylergannon/skgo => "+repo), 1)
	if err := os.WriteFile(mod, data, 0644); err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func TestWebGrammarUnionsAndWrappedPointers(t *testing.T) {
	source := strings.Replace(nativeModelSource, `Node:Node{Name:"leaf"}`, `Node:Node{Name:"root",Next:polytype.Nullable[*Node]{Present:true,Value:&Node{Name:"leaf"}}}`, 1)
	root, cfg := nativeModelFixture(t, t.TempDir(), source)
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	ts := readFixtureFile(t, root, "app/web/src/data/types.ts")
	for _, pattern := range []string{`Omit<Note,\s*["']kind["']>`, `["']?kind["']?\s*:\s*["']Note["']`, `["']?Next["']?\s*:\s*Node\s*\|\s*null`} {
		if !regexp.MustCompile(pattern).MatchString(ts) {
			t.Fatalf("missing TypeScript contract %s:\n%s", pattern, ts)
		}
	}
	bindings := readFixtureFile(t, root, "app/generated/skgo_gen.go")
	if strings.Contains(bindings, "skgoNativeCheck") {
		t.Fatal("web-only generation adopted native numeric guards")
	}
	writeSharedFixture(t, filepath.Join(root, "app"), "generated/web_test.go", webGrammarHandlerTests)
	cmd := exec.Command("go", "test", "-count=1", "./generated")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("web-only real handler: %v\n%s", err, out)
	}
}

const webGrammarHandlerTests = `package generated_test
import("encoding/json";"net/http/httptest";"strings";"testing";"github.com/tylergannon/devalue/v5";"github.com/tylergannon/skgo";"example.com/app/generated")
func TestWebUnionAndRecursiveValue(t *testing.T){
 remotes:=generated.Remotes();h,err:=skgo.NewRemotes(skgo.RemoteConfig{},remotes...);if err!=nil {t.Fatal(err)}
 var id string;for _,remote:=range remotes{if strings.HasSuffix(remote.ID(),"/read"){id=remote.ID()}};if id==""{t.Fatal("missing query")}
 w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET","http://web.test/_app/remote/"+id,nil))
 var envelope struct{Type,Data string};if err:=json.Unmarshal(w.Body.Bytes(),&envelope);err!=nil{t.Fatal(err)}
 if w.Code!=200 || envelope.Type!="result"{t.Fatalf("%d %s",w.Code,w.Body.String())}
 parsed,err:=devalue.Parse(envelope.Data,nil);if err!=nil {t.Fatal(err)}
 get:=func(object *devalue.Object,key string)any{value,ok:=object.Get(key);if !ok{t.Fatal("missing",key)};return value}
 object:=get(parsed.(*devalue.Object),"_").(*devalue.Object)
 choice:=get(object,"Choice").(*devalue.Object);if get(choice,"kind")!="Note" || get(choice,"Text")!="hello" {t.Fatalf("union=%v",choice)}
 node:=get(object,"Node").(*devalue.Object);if get(node,"Name")!="root" {t.Fatal("missing root")}
 leaf:=get(node,"Next").(*devalue.Object);if get(leaf,"Name")!="leaf" || get(leaf,"Next")!=nil {t.Fatalf("leaf=%v",leaf)}
}
`

func TestNativeSwiftGenerationAndGoHandlerDomain(t *testing.T) {
	root, cfg := generatedNativeFixture(t)
	first, err := os.ReadFile(cfg.SwiftOut)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := skgoBinary()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"generate", "--quiet", "--web", cfg.Web, "--out", cfg.Out, "--locals-package", cfg.LocalsPackage, "--swift-out", cfg.SwiftOut}
	for _, selection := range cfg.SwiftRemotes {
		args = append(args, "--swift-remote", selection)
	}
	generate := exec.Command(bin, args...)
	generate.Dir = filepath.Join(root, "app")
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("native generation CLI: %v\n%s", err, out)
	}
	second, err := os.ReadFile(cfg.SwiftOut)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Swift generation is not deterministic")
	}
	writeSharedFixture(t, filepath.Join(root, "app"), "generated/native_test.go", nativeHandlerTests)
	cmd := exec.Command("go", "test", "-count=1", "./generated")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated real handler: %v\n%s", err, out)
	}
}

const nativeHandlerTests = `package generated_test
import("bytes";"encoding/base64";"encoding/json";"net/http/httptest";"strings";"testing";"github.com/tylergannon/skgo";"example.com/app/generated";"example.com/app/web/src/data")
func TestNativeNumberDomain(t *testing.T){
 remotes:=generated.Remotes()
 h,err:=skgo.NewRemotes(skgo.RemoteConfig{Origin:"http://native.test"},remotes...);if err!=nil {t.Fatal(err)}
 for _,tc:=range []struct{name,method,argument,wantData string;status,wideCalls,signedCalls,numberCalls int}{
 {name:"wide",method:"GET",status:500},
 {name:"wideInput",method:"POST",argument:"[9007199254740991]",wantData:` + "`" + `[{"_":1},9007199254740991]` + "`" + `,wideCalls:1},
 {name:"wideInput",method:"POST",argument:"[9007199254740992]",status:400,wideCalls:1},
 {name:"signedInput",method:"POST",argument:"[-9007199254740991]",wantData:` + "`" + `[{"_":1},-9007199254740991]` + "`" + `,wideCalls:1,signedCalls:1},
 {name:"signedInput",method:"POST",argument:"[-9007199254740992]",status:400,wideCalls:1,signedCalls:1},
 {name:"signedInput",method:"POST",argument:"[9007199254740992]",status:400,wideCalls:1,signedCalls:1},
 {name:"wideSigned",method:"GET",status:500,wideCalls:1,signedCalls:1},
 {name:"numberInput",method:"POST",argument:"[1.25]",wantData:` + "`" + `[{"_":1},1.25]` + "`" + `,wideCalls:1,signedCalls:1,numberCalls:1},
 {name:"numberInput",method:"POST",argument:"-3",status:400,wideCalls:1,signedCalls:1,numberCalls:1},
 {name:"numberInput",method:"POST",argument:"-4",status:400,wideCalls:1,signedCalls:1,numberCalls:1},
 {name:"numberInput",method:"POST",argument:"-5",status:400,wideCalls:1,signedCalls:1,numberCalls:1},
 {name:"wideNumber",method:"GET",status:500,wideCalls:1,signedCalls:1,numberCalls:1},
 }{
 var id string;for _,remote:=range remotes {if strings.HasSuffix(remote.ID(),"/"+tc.name){id=remote.ID()}}
 if id==""{t.Fatal("missing selected remote",tc.name)}
 var body []byte
 if tc.method=="POST" {body,err=json.Marshal(map[string]any{"payload":base64.RawURLEncoding.EncodeToString([]byte(tc.argument)),"refreshes":[]string{}});if err!=nil{t.Fatal(err)}}
 req:=httptest.NewRequest(tc.method,"http://native.test/_app/remote/"+id,bytes.NewReader(body))
 req.Header.Set("Origin","http://native.test");req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder();h.ServeHTTP(w,req)
 var envelope map[string]any;if err:=json.Unmarshal(w.Body.Bytes(),&envelope);err!=nil {t.Fatal(err)}
 if tc.status==0 {
 if envelope["type"]!="result" || envelope["data"]!=tc.wantData {t.Fatalf("safe boundary did not round trip: %d %s",w.Code,w.Body.String())}
 } else {
 failure,ok:=envelope["error"].(map[string]any)
 if envelope["type"]!="error" || !ok || failure["status"]!=float64(tc.status) {t.Fatalf("%s emitted unsafe number: %d %s",tc.name,w.Code,w.Body.String())}
 }
 if data.WideCalls!=tc.wideCalls || data.SignedCalls!=tc.signedCalls || data.NumberCalls!=tc.numberCalls {t.Fatalf("%s application calls=(%d,%d,%d); want (%d,%d,%d)",tc.argument,data.WideCalls,data.SignedCalls,data.NumberCalls,tc.wideCalls,tc.signedCalls,tc.numberCalls)}
 }
}
`

func TestNativeSelectionAndOutputAdmission(t *testing.T) {
	for _, tc := range []struct {
		name       string
		selections []string
		output     string
		want       string
	}{
		{"unknown", []string{"src/data/data.remote.ts#missing"}, "Generated.swift", "unknown Swift remote"},
		{"duplicate", []string{"src/data/data.remote.ts#read", "src/data/data.remote.ts#read"}, "Generated.swift", "duplicate Swift selection"},
		{"authored", []string{"src/data/data.remote.ts#read"}, "Authored.swift", "authored"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, cfg := foreignFixture(t, `package data;import("context";"github.com/tylergannon/skgo");func read(context.Context)(string,error){return "",nil};var _=skgo.Query(read)`, nil)
			cfg.SwiftRemotes = tc.selections
			cfg.SwiftOut = filepath.Join(root, "app", tc.output)
			writeSharedFixture(t, filepath.Join(root, "app"), tc.output, "// Authored\n")
			if tc.name != "authored" {
				if err := os.Remove(cfg.SwiftOut); err != nil {
					t.Fatal(err)
				}
			}
			err := Run(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v; want %s", err, tc.want)
			}
			if tc.name == "authored" {
				got, _ := os.ReadFile(cfg.SwiftOut)
				if string(got) != "// Authored\n" {
					t.Fatal("authored output changed")
				}
			}
		})
	}
	for _, cfg := range []Config{{SwiftOut: "Generated.swift"}, {SwiftRemotes: []string{"x#read"}}} {
		if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "provided together") {
			t.Fatalf("error=%v", err)
		}
	}
}

func TestNativeUnsupportedKindsAndTransports(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		extra              map[string]string
	}{
		{"live", `package data;import("context";"github.com/tylergannon/skgo");func read(context.Context,func(string)error)error{return nil};var _=skgo.LiveQuery(read)`, "native Swift supports ordinary queries and commands", nil},
		{"transport", `package data;import("context";"github.com/tylergannon/skgo";hooks "example.com/app/web/src");func read(context.Context)(hooks.Money,error){return hooks.Money{},nil};var _=skgo.Query(read)`, "custom transport unsupported", map[string]string{"app/web/src/hooks.go": `package hooks;import "github.com/tylergannon/skgo";type Money struct{Cents int};var _=skgo.Transported[Money]("Money")`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, cfg := foreignFixture(t, tc.source, tc.extra)
			cfg.SwiftOut = filepath.Join(root, "app", "Generated.swift")
			cfg.SwiftRemotes = []string{"src/data/data.remote.ts#read"}
			err := Run(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "data.remote.go:") {
				t.Fatalf("error=%v", err)
			}
			if _, err := os.Stat(cfg.SwiftOut); !os.IsNotExist(err) {
				t.Fatal("unsupported shape published Swift output")
			}
		})
	}
}

const nativeServerSource = `package main
import("encoding/json";"fmt";"net";"net/http";"sync";"github.com/tylergannon/skgo";"example.com/app/generated")
func main(){
 listener,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{panic(err)}
 origin:="http://"+listener.Addr().String()
 h,err:=skgo.NewRemotes(skgo.RemoteConfig{Origin:origin},generated.Remotes()...);if err!=nil{panic(err)}
 counts:=map[string]int{};var mu sync.Mutex
 handler:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 if r.URL.Path=="/counts"{mu.Lock();defer mu.Unlock();json.NewEncoder(w).Encode(counts);return}
 mu.Lock();counts[r.Method+" "+r.URL.Path]++;mu.Unlock();h.ServeHTTP(w,r)
 })
 fmt.Println(origin);if err:=http.Serve(listener,handler);err!=nil{panic(err)}
}
`
