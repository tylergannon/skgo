package ssr_test

import (
	"context"
	"testing"

	"github.com/tylergannon/skgo/internal/ssr"
)

func TestEnvironmentExistsBeforeTopLevelApplicationEvaluation(t *testing.T) {
	const source = `
 const enabled = globalThis.__skgo_environment.dynamicPublic.ENABLED;
 const count = globalThis.__skgo_environment.dynamicPublic.COUNT;
 const secret = globalThis.__skgo_environment.private.SECRET;
 if (typeof enabled !== 'boolean' || typeof count !== 'number') throw new Error('untyped environment');
 globalThis.__skgo_ping = () => 'ok';
 globalThis.__skgo_render = () => ({done:true,status:200,body: enabled + ':' + count + ':' + secret});`
	environment := []byte(`{"dynamicPublic":{"ENABLED":false,"COUNT":123},"private":{"SECRET":"server-receipt"}}`)
	engine, err := ssr.New("environment.js", []byte(source), 2, nil, environment)
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := engine.Render(context.Background(), "/", []byte(`{}`), ssr.Hosts{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Body != "false:123:server-receipt" {
		t.Fatalf("top-level snapshot: %q", result.Body)
	}
}
