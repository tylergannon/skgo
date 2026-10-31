// Command example prepares the Windows shell consumer or runs its synthetic
// browser and authenticated SKGo services. It needs no external account.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/plugins/windows-shell/csharpgen"
	"github.com/tylergannon/skgo/plugins/windows-shell/windowsapp"
	"github.com/tylergannon/skgo/templateapi"
)

//go:embed ApplicationView.cs
var applicationView []byte

func main() {
	prepare := flag.String("prepare", "", "Create the consumer in this directory and cross-compile its DLL")
	api := flag.String("api", "127.0.0.1:48321", "Synthetic authenticated native service")
	web := flag.String("web", "127.0.0.1:48322", "Independent synthetic browser service")
	flag.Parse()
	if *prepare != "" {
		root, err := filepath.Abs(*prepare)
		must(err)
		_, priorView := os.Stat(filepath.Join(root, "windows/ApplicationView.cs"))
		_, err = windowsapp.Apply(context.Background(), templateapi.Request{Addon: "windows-app", Project: templateapi.Project{Root: root}, Options: map[string]string{"identity": "SKGo.WindowsExample"}})
		must(err)
		// Example setup is also create-only. This authored view replaces the
		// default only during the explicit example preparation command.
		if os.IsNotExist(priorView) {
			must(os.WriteFile(filepath.Join(root, "windows/ApplicationView.cs"), applicationView, 0644))
		}
		query, command := operations()
		generated, err := csharpgen.Generate(nil, []tg.Type{&tg.Scalar{Kind: tg.String}}, []csharpgen.Call{
			{Name: "getValue", ID: query.ID(), Kind: "query", Input: -1, Output: 0},
			{Name: "setValue", ID: command.ID(), Kind: "command", Input: 0, Output: 0},
		})
		must(err)
		must(os.WriteFile(filepath.Join(root, "windows/Generated.cs"), generated.Source, 0644))
		must(windowsapp.Prepare(context.Background(), root, os.Stdout))
		return
	}
	query, command := operations()
	_, webPort, err := net.SplitHostPort(*web)
	must(err)
	remotes, err := skgo.NewRemotes(skgo.RemoteConfig{Origin: "http://" + *api, Version: "windows-shell-example"}, query, command)
	must(err)
	native := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-native-token" {
			http.Error(w, "Native authentication required", http.StatusUnauthorized)
			return
		}
		log.Printf("native authenticated %s %s", r.Method, r.URL.Path)
		remotes.ServeHTTP(w, r)
	})
	browser := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "Native credentials leaked into browser", 500)
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://localhost:"+webPort+"/second", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><title>SKGo embedded page</title></head><body style="font:18px system-ui;padding:36px;background:#eef3f8;color:#17344c"><h1>SKGo in WebView2</h1><p id="extension">Waiting for the application's document script.</p><p>This browser has its own origin and receives no native API credentials.</p><p><a href="/redirect">Follow an allowed redirect</a> · <a href="http://127.0.0.1:48323/">Try an unlisted origin</a> · <a target="_blank" href="/second">Open allowed popup in this view</a></p><button onclick="chrome.webview.postMessage({kind:'example',text:'Synthetic web event'})">Send native event</button></body></html>`)
	})
	go func() { log.Fatal(http.ListenAndServe(*web, browser)) }()
	log.Printf("native=http://%s browser=http://%s", *api, *web)
	log.Fatal(http.ListenAndServe(*api, native))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func operations() (*skgo.Remote, *skgo.Remote) {
	var mu sync.Mutex
	value := "Windows native query"
	query := skgo.NewRemote(skgo.RemoteSpec{Module: "src/windows.remote.ts", Name: "getValue", Kind: skgo.KindQuery, Call: func(context.Context, skgo.Call) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		return value, nil
	}})
	command := skgo.NewRemote(skgo.RemoteSpec{Module: "src/windows.remote.ts", Name: "setValue", Kind: skgo.KindCommand, Call: func(_ context.Context, call skgo.Call) (any, error) {
		text, ok := call.Arg.(string)
		if !ok {
			return nil, skgo.Errorf(400, "Expected text")
		}
		mu.Lock()
		defer mu.Unlock()
		value = text
		return value, nil
	}})
	return query, command
}
