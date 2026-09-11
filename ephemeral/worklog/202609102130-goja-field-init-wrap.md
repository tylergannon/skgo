finding: #117 (class-field `$derived` panics goja) is a goja bug — `_initFields` inherits `vm.args` from the function running `new`, so an initialiser that closes over `this` gets a shifted stack base. Upstream fix: dop251/goja#737; skgo#138 carries the regression test and the bump.

decision: until skgo requires a goja with #737, the adapter's `wrapFieldInitialisers` (skgo-adapter/env.js) rewrites such initialisers to `(function () { return VALUE; }).call(this)`, which reads `this` without capturing it. Private fields stay native — es2021 lowering also fixes it but turns them into WeakMaps, which SSR_TARGET exists to avoid.

trap: when goja is bumped past #737, delete `wrapFieldInitialisers` and its call. `TestARunesClassWithAClassFieldDerivedRenders` (example/ssr_test.go) keeps guarding the behaviour either way. Initialisers reaching `super` are left unwrapped and still hit the goja bug until then.
