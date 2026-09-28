package adapter

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrivateEnvironmentMiddlewareDeniesViteRawServerModules(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs("skgo-adapter/env-private.js")
	if err != nil {
		t.Fatal(err)
	}
	const source = `
import {pathToFileURL} from 'node:url';
import {mkdirSync,writeFileSync,symlinkSync,existsSync} from 'node:fs';
import {join} from 'node:path';
const {protectEnvironmentFiles} = await import(pathToFileURL(process.argv[1]));
const protect = protectEnvironmentFiles({root:'/fixture/web',kitOut:'.svelte-kit',output:'build',token:'fixture-token'});
const privatePaths = [
 '/.svelte-kit/output/server/chunks/config.js',
 '/.svelte-kit/generated/build/env/private/server.js',
 '/.svelte-kit/generated/dev/env/private/server.js',
 '/.svelte-kit/generated/build/env/config.js',
 '/.svelte-kit/generated/dev/env/config.js',
 '/.svelte-kit/skgo-env-values.js',
 '/.svelte-kit/skgo-env-runtime.json',
 '/build/env.json', '/build/ssr/bundle.js'
];
function check(url, want, headers={}) {
 let status = 200;
 const res = {statusCode:200,end(){status=this.statusCode}};
 protect({url,headers},res,()=>{});
 if (status !== want) throw new Error(url + ': expected ' + want + ', got ' + status);
}
for (const path of privatePaths) {
 for (const query of ['', '?raw', '?import']) {
  check(path + query, 403);
  check('/@fs/fixture/web' + path + query,403);
  check('/@id//fixture/web' + path + query,403);
  check(path.replace('.svelte-kit','%2Esvelte-kit') + query,403);
 }
}
check('/@id/$app/env/private?raw',403);
check('/@id/__x00__<sveltekit:generated>/env/config.js?raw',403);
check('/__skgo_dev/module',403);
check('/__skgo_dev/module/extra',403);
check('/__skgo_dev/module',200,{'x-skgo-dev-token':'fixture-token'});
for (const path of ['/.svelte-kit/generated/dev/env/public/client.js','/.svelte-kit/generated/dev/client/app.js','/src/routes/+page.svelte','/@vite/client']) check(path,200);
const root = process.argv[2];
const secret = join(root,'.svelte-kit/generated/dev/env/private/server.js');
mkdirSync(join(root,'.svelte-kit/generated/dev/env/private'),{recursive:true});
writeFileSync(secret,'private-sentinel');
symlinkSync(secret,join(root,'alias.js'));
const realProtect = protectEnvironmentFiles({root,kitOut:'.svelte-kit',output:'build'});
for (const path of ['/alias.js', ...existsSync(join(root,'.SVELTE-KIT')) ? ['/.SVELTE-KIT/generated/dev/env/private/server.js','/.svelte-kit/generated/dev/ENV/private/server.js'] : []]) {
 const res = {statusCode:200,end(){}};
 realProtect({url:path+'?raw',headers:{}},res,()=>{});
 if (res.statusCode !== 403) throw new Error('filesystem alias bypass: '+path);
}
`
	cmd := exec.Command(node, "--input-type=module", "-e", source, module, t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("private environment HTTP boundary: %v\n%s", err, out)
	}
}
