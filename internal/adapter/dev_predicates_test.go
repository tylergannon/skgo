package adapter

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativePredicatesReachGoWhileMainThreadWaitsAndRecoverAfterOwnerLoss(t *testing.T) {
	dir, err := os.MkdirTemp("", "skgo-predicate-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "go.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_skgo/dev/resolve-option" || r.Header.Get("Authorization") != "Bearer literal-secret" {
			http.Error(w, "wrong bridge", 400)
			return
		}
		var input struct {
			Handle string `json:"handle"`
			Option string `json:"option"`
			Name   string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid", 400)
			return
		}
		if input.Handle == "old-owner" {
			http.Error(w, "unknown owner", 409)
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"value": input.Handle == "literal-owner" && input.Option == "filter" && input.Name == "x-literal-447"})
	})}
	go server.Serve(listener)
	defer server.Close()
	worker, err := filepath.Abs("skgo-adapter/dev-predicates.js")
	if err != nil {
		t.Fatal(err)
	}
	const program = `
import assert from 'node:assert/strict';
import {Worker,MessageChannel,receiveMessageOnPort} from 'node:worker_threads';
import {pathToFileURL} from 'node:url';
const {port1,port2}=new MessageChannel();
const worker=new Worker(pathToFileURL(process.argv[1]),{workerData:{port:port2},execArgv:[],transferList:[port2]});
function call(id,handle){
 const cell=new Int32Array(new SharedArrayBuffer(4));
 port1.postMessage({id,cell,payload:{handle,option:'filter',name:'x-literal-447'}});
 assert.equal(Atomics.wait(cell,0,0,5000),'ok');
 const reply=receiveMessageOnPort(port1).message;
 assert.equal(reply.id,id);return reply;
}
assert.equal(call(1,'literal-owner').value,true);
assert.match(call(2,'old-owner').error,/HTTP 409/);
assert.equal(call(3,'literal-owner').value,true);
await worker.terminate();port1.close();
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, worker)
	command.Env = append(os.Environ(), "SKGO_DEV_GO_SOCKET="+socket, "SKGO_DEV_SERVICE_SECRET=literal-secret")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("synchronous Go predicate relay: %v\n%s", err, output)
	}
}
