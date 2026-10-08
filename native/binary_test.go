package native_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo"
)

// Recorded from Kit 3.0.0 runtime/shared.js and its resolved devalue 5.9.4.
// Go and Zig independently construct/inspect graphs; neither writes expectations.
//
//go:embed core/src/kit-binary.json
var binaryFixture []byte

var binaryKinds = []devalue.TypedArrayKind{
	devalue.Int8Array, devalue.Uint8Array, devalue.Uint8ClampedArray,
	devalue.Int16Array, devalue.Uint16Array, devalue.Float16Array,
	devalue.Int32Array, devalue.Uint32Array, devalue.Float32Array,
	devalue.Float64Array, devalue.BigInt64Array, devalue.BigUint64Array,
}

func binaryGraph(raw []byte) *devalue.Object {
	buffer := devalue.NewArrayBuffer(raw)
	view := &devalue.TypedArray{Kind: devalue.Uint8Array, Buffer: buffer, ByteOffset: 1, ByteLength: 3}
	empty := devalue.NewArrayBuffer(nil)
	kinds := make([]any, len(binaryKinds))
	for i, kind := range binaryKinds {
		width := kind.BytesPerElement()
		kinds[i] = &devalue.TypedArray{Kind: kind, Buffer: buffer, ByteOffset: width, ByteLength: width}
	}
	root := devalue.NewObject("view", view, "again", view,
		"equal", &devalue.TypedArray{Kind: devalue.Uint8Array, Buffer: buffer, ByteOffset: 1, ByteLength: 3},
		"data", &devalue.DataView{Buffer: buffer, ByteOffset: 2, ByteLength: 4},
		"buffer", buffer, "empty", empty, "emptyView", devalue.NewTypedArray(devalue.Uint8Array, empty),
		"separateEmpty", devalue.NewArrayBuffer(nil), "kinds", kinds)
	root.Set("self", root)
	return root
}

func checkBinaryGraph(value any, want []byte) error {
	root, ok := value.(*devalue.Object)
	if !ok {
		return fmt.Errorf("root = %T; want object", value)
	}
	get := func(key string) any { v, _ := root.Get(key); return v }
	buffer, ok := get("buffer").(devalue.ArrayBuffer)
	if !ok || !bytes.Equal(buffer, want) {
		return fmt.Errorf("backing buffer = %v; want %v", get("buffer"), want)
	}
	sameBuffer := func(b devalue.ArrayBuffer) bool {
		return reflect.ValueOf(b).Pointer() == reflect.ValueOf(buffer).Pointer()
	}
	view, ok := get("view").(*devalue.TypedArray)
	if !ok || view.Kind != devalue.Uint8Array || view.ByteOffset != 1 || view.ByteLength != 3 || !sameBuffer(view.Buffer) {
		return fmt.Errorf("invalid subview: %v", get("view"))
	}
	equal, ok := get("equal").(*devalue.TypedArray)
	if !ok || equal == view || equal.Kind != view.Kind || equal.ByteOffset != 1 || equal.ByteLength != 3 || !sameBuffer(equal.Buffer) || get("again") != view {
		return fmt.Errorf("repeated/distinct view identity or geometry lost")
	}
	data, ok := get("data").(*devalue.DataView)
	if !ok || data.ByteOffset != 2 || data.ByteLength != 4 || !sameBuffer(data.Buffer) {
		return fmt.Errorf("DataView geometry or shared buffer lost")
	}
	empty, ok := get("empty").(devalue.ArrayBuffer)
	separate, separateOK := get("separateEmpty").(devalue.ArrayBuffer)
	emptyView, viewOK := get("emptyView").(*devalue.TypedArray)
	if !ok || !separateOK || !viewOK || len(empty) != 0 || len(separate) != 0 || emptyView.Kind != devalue.Uint8Array || emptyView.ByteOffset != 0 || emptyView.ByteLength != 0 ||
		reflect.ValueOf(empty).Pointer() == reflect.ValueOf(separate).Pointer() || reflect.ValueOf(emptyView.Buffer).Pointer() != reflect.ValueOf(empty).Pointer() {
		return fmt.Errorf("empty buffer ownership or empty view lost")
	}
	if get("self") != root {
		return fmt.Errorf("root cycle lost")
	}
	kinds, ok := get("kinds").([]any)
	if !ok || len(kinds) != 12 {
		return fmt.Errorf("typed kinds = %T; want twelve views", get("kinds"))
	}
	for i, kind := range binaryKinds {
		v, ok := kinds[i].(*devalue.TypedArray)
		if !ok || v.Kind != kind || v.ByteOffset != kind.BytesPerElement() || v.ByteLength != kind.BytesPerElement() || !sameBuffer(v.Buffer) {
			return fmt.Errorf("%s geometry or shared buffer lost: %v", kind, kinds[i])
		}
	}
	return nil
}

func testZigBinaryCalls(t *testing.T, ctx context.Context, cli string) {
	t.Helper()
	var fixture struct{ Kit, Devalue, Input, Result string }
	if err := json.Unmarshal(binaryFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Kit != "3.0.0" || fixture.Devalue != devalue.UpstreamVersion {
		t.Fatalf("fixture versions = %s/%s", fixture.Kit, fixture.Devalue)
	}
	for _, kind := range []skgo.Kind{skgo.KindQuery, skgo.KindCommand} {
		name := "query"
		if kind == skgo.KindCommand {
			name = "command"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })
			origin := "http://" + listener.Addr().String()
			var calls atomic.Int32
			remote := skgo.NewRemote(skgo.RemoteSpec{Module: "src/binary.remote.ts", Name: "exchange", Kind: kind,
				Call: func(_ context.Context, call skgo.Call) (any, error) {
					calls.Add(1)
					if !call.Present {
						return nil, skgo.Errorf(400, "Missing binary argument")
					}
					if err := checkBinaryGraph(call.Arg, []byte{0, 255, 1, 128, 254, 127, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}); err != nil {
						return nil, skgo.Errorf(400, "%s", err)
					}
					// A fresh Go graph with different bytes proves a response was decoded,
					// rather than merely reflecting the client's input document.
					return binaryGraph([]byte{11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 128, 254, 255, 0}), nil
				},
			})
			handler, err := skgo.NewRemotes(skgo.RemoteConfig{Base: "/native", Origin: origin}, remote)
			if err != nil {
				t.Fatal(err)
			}
			server := &http.Server{Handler: handler}
			go server.Serve(listener)
			t.Cleanup(func() { server.Close() })
			out, err := exec.CommandContext(ctx, cli, origin, "/native", remote.ID(), name, fixture.Input).CombinedOutput()
			if err != nil {
				t.Fatalf("Zig %s HTTP exchange: %v\n%s", name, err, out)
			}
			got := strings.TrimSpace(string(out))
			if got != fixture.Result {
				t.Fatalf("Zig response = %s; want upstream %s", got, fixture.Result)
			}
			value, err := devalue.Parse(got, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkBinaryGraph(value, []byte{11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 128, 254, 255, 0}); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatalf("Go handler calls = %d; want 1", calls.Load())
			}
		})
	}
}
