package adapter

import "testing"

// Kit calls the app's reroute hook with its own fetch; the shipped hook asks
// the Go reroute endpoint for the path. Kit replaces the two path globals at
// build time, so they are set here before the module loads.
func TestRerouteHookAsksGoForThePath(t *testing.T) {
	t.Parallel()
	const program = `import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
globalThis.__SVELTEKIT_PATHS_BASE__='/base';
globalThis.__SVELTEKIT_APP_DIR__='_app';
const {reroute}=await import(pathToFileURL(process.argv[1]).href);
function go(status,body) {
 const fetched=[];
 return {fetched,fetch:async url=>{fetched.push(url);return new Response(body,{status,headers:{'content-type':'application/json'}})}};
}

const alias=go(200,'{"path":"/destination"}');
assert.equal(await reroute({url:new URL('https://fixture.test/alias?x=1'),fetch:alias.fetch}),'/destination','the path Go chose');
assert.deepEqual(alias.fetched,['/base/_app/reroute?url=https%3A%2F%2Ffixture.test%2Falias%3Fx%3D1'],'one request to the Go reroute endpoint');

const endpoint=go(200,'{"path":"/destination"}');
assert.equal(await reroute({url:new URL('https://fixture.test/base/_app/reroute'),fetch:endpoint.fetch}),undefined,'the endpoint itself is not rerouted');
assert.deepEqual(endpoint.fetched,[],'the endpoint itself is not sent to Go');

const failed=go(503,'');
await assert.rejects(reroute({url:new URL('https://fixture.test/alias'),fetch:failed.fetch}),{message:'skgo: reroute failed (503)'});
assert.deepEqual(failed.fetched,['/base/_app/reroute?url=https%3A%2F%2Ffixture.test%2Falias']);
`
	runAdapterScript(t, "skgo-adapter/reroute.js", program, nil)
}
