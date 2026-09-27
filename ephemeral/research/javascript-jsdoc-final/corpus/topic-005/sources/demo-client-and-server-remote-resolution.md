# Demo probe: how kit's client and server resolve a remote function at runtime
# origin: /tmp/skgo-jsdoc-demo-probe-20260926/web (built 2026-09-26 10:55)
# retrieved: 2026-09-26 (file reads only; no build or test was run)
# location: build/ (shipped) and .svelte-kit/output/ (pre-adapter) trees; line numbers
#           are from `cat -n` of each file at retrieval.
# --- verbatim ---

## 1. The app entry does NOT name remote functions

`build/client/_app/immutable/entry/app.DyKfhsWA.js` (line 1 is the dep map,
line 2 the whole module, reproduced verbatim):

```js
const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["../nodes/0.RW1BpA4A.js","../chunks/B_JM58MY.js","../nodes/1.CQiYzX7O.js","../chunks/CstUhczG.js","./payload.DSmR2FwN.js","../nodes/2.CXgr7ugK.js","../chunks/D-0zb02F.js","../assets/2.DktYJJwg.css"])))=>i.map(i=>d[i]);
import{t as e}from"../chunks/HclGiUj8.js";var t={},n=[()=>e(()=>import(`../nodes/0.RW1BpA4A.js`),__vite__mapDeps([0,1]),import.meta.url),()=>e(()=>import(`../nodes/1.CQiYzX7O.js`),__vite__mapDeps([2,1,3,4]),import.meta.url),()=>e(()=>import(`../nodes/2.CXgr7ugK.js`),__vite__mapDeps([5,1,3,4,6,7]),import.meta.url)],r=[],i={"/":[2]},a={handleError:(({kind:e,error:t})=>{e===`unknown`&&console.error(t)}),reroute:(()=>{}),transport:{}},o=Object.fromEntries(Object.entries(a.transport).map(([e,t])=>[e,t.decode])),s=Object.fromEntries(Object.entries(a.transport).map(([e,t])=>[e,t.encode])),c=!1,l=(e,t)=>o[e](t),u=()=>e(()=>import(`../chunks/Bjy-W4x2.js`).then(e=>e.default),[],import.meta.url);export{l as decode,o as decoders,i as dictionary,s as encoders,u as get_error_template,c as hash,a as hooks,t as matchers,n as nodes,r as server_loads};
```

Observed facts about this module: its exports are `decode, decoders, dictionary,
encoders, get_error_template, hash, hooks, matchers, nodes, server_loads`;
`server_loads` is the empty array `r=[]`; there is no `remotes` key and no
remote id or remote chunk import anywhere in the entry. `grep -o "remote[^,\`]*"`
over `entry/start.VuppPwFt.js` returned nothing. The literal string `1ptltty`
does not occur in `entry/app.DyKfhsWA.js`.

## 2. The route node carries the remote ids as string literals

`build/client/_app/immutable/nodes/2.CXgr7ugK.js` (single minified line).
Verbatim excerpts of the fragments that bind and use the ids:

```js
var V=Symbol(`sveltekit.query_function_id`),H=Symbol(`sveltekit.query_override_key`),U=Symbol(`sveltekit.query_resource_key`);
```

```js
function ge(e){let t=f(0),n=n=>{let r=null,i=null,o;a(t);let s={"Content-Type":`application/json`,...K()},c=(async()=>{try{if(await Promise.resolve(),o)throw o;let t=await q(`${D}/${M}/remote/${e}`,{method:`POST`,body:JSON.stringify({payload:await le(n),refreshes:Array.from(i??[])}),headers:s},i);
```

```js
function ye(e){let t=t=>new ve(e,t,async t=>{let n=await q(`${D}/${M}/remote/${e}${t?`?payload=${t}`:``}`);n.redirect&&await pe(n.redirect)});return Object.defineProperty(t,V,{value:e}),t}
```

```js
var be=ge(`1ptltty/record`),$=ye(`1ptltty/status`),
```

So the client half of a remote is: an id literal `<hash>/<name>`, and an HTTP
call to `` `${D}/${M}/remote/${id}` `` where `D` and `M` are imported from the
route chunk's shared chunk (see next item). No client-side dynamic import of a
remote chunk appears in any shipped client file:
`grep -rln "remote" build/client/_app/immutable/` matched only
`nodes/2.CXgr7ugK.js`, `chunks/D-0zb02F.js`, `chunks/B_JM58MY.js`.

The same literals appear in the shipped file:

    build/client/_app/immutable/nodes/2.CXgr7ugK.js:`1ptltty/record`
    build/client/_app/immutable/nodes/2.CXgr7ugK.js:`1ptltty/status`

## 3. What D and M are

`build/client/_app/immutable/chunks/CstUhczG.js` head (verbatim):

```js
import{payload as e}from"../entry/payload.DSmR2FwN.js";import{N as t,P as n,S as r,y as i}from"./B_JM58MY.js";var a=e.base??``,o=e.assets??a??``,s=`_app`;
```

Its export line (verbatim, tail of file):

```js
export{C as a,v as c,a as d,c as f,E as i,m as l,N as n,M as o,w as r,x as s,D as t,s as u}
```

`nodes/2.CXgr7ugK.js` imports `{d as D,...,u as M}` from this chunk, i.e. `D` is
the module-local `a` (= `payload.base ?? ''`) and `M` is the module-local `s`
(= `'_app'`). The resolved request path is therefore
`${base}/${appDir}/remote/<hash>/<name>`.

## 4. The server side: manifest keyed by module hash → chunk

`.svelte-kit/output/server/manifest.js` lines 19-21 (verbatim):

```js
		remotes: {
			'1ptltty': __memo(() => import('./chunks/remote-1ptltty.js'))
		},
```

(the same map appears at line 20 of `manifest-full.js`).

`.svelte-kit/output/server/chunks/remote-1ptltty.js` (whole file):

```js
import { t as example_remote_exports } from "./example.remote.js";
//#region \0sveltekit-remote:1ptltty
var _sveltekit_remote_1ptltty_default = example_remote_exports;
//#endregion
export { _sveltekit_remote_1ptltty_default as default };
```

`.svelte-kit/output/server/chunks/example.remote.js` lines 16-31 (verbatim,
after the rolldown runtime region):

```js
//#region src/routes/example.remote.ts
var example_remote_exports = /* @__PURE__ */ __exportAll({
	record: () => record,
	status: () => status
});
var unimplemented = () => {
	throw new Error("skgo: implemented in Go");
};
var record = /* @__PURE__ */ command("unchecked", (_arg) => unimplemented());
var status = /* @__PURE__ */ query(() => unimplemented());
init_remote_functions(example_remote_exports, "src/routes/example.remote.ts", "1ptltty");
for (const [name, fn] of Object.entries(example_remote_exports)) {
	fn.__.id = "1ptltty/" + name;
	fn.__.name = name;
}
//#endregion
```

The module hash `1ptltty` is the hash of the module path
`src/routes/example.remote.ts` (extension included), passed to
`init_remote_functions` alongside the path, and the per-function id is
`"<hash>/" + name`.

`.svelte-kit/output/server/.vite/manifest.json` (the corresponding client/server
rollup entry is not present for remotes in the client manifest —
`grep -n remote .svelte-kit/output/client/.vite/manifest.json` returned no
matches; the remote chunk exists only in the server output).

## 5. What the app's own manifest records

`web/build/skgo.manifest.json` lines 114-117 (verbatim):

```json
	"remotes": [
		"1ptltty/record",
		"1ptltty/status"
	],
```

Identity lines 2-3: `"skgo": "0.9.0"`, `"skgoAdapter": "17f12e0497d6"`.
