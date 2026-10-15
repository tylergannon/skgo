import { sveltekit } from '@sveltejs/kit/vite';
import { createBuilder, createServer } from 'vite';
import { resolve } from 'node:path';
import { writeFileSync, readFileSync } from 'node:fs';
import { gojaEnvironment, nodeTable } from 'ADAPTER_ENV';
const goja = gojaEnvironment();
const nodes = nodeTable(process.cwd(), [{component:'src/routes/+layout.svelte'}, {component:'src/routes/+page.svelte'}]);
const generated = {nodes, hooks:'export const transport = {};'};
const adapter = {
 name:'renderer-experiment',
 vite:{plugins:{post:[goja.plugin]}},
 async adapt() {
  await goja.build(generated, 'bundle.js');
  globalThis.__rendererPlugin = goja.plugin;
  globalThis.__rendererRebuild = () => goja.build(generated, 'bundle.js');
 }
};
export default {
 plugins:[sveltekit({adapter, paths:{origin:'http://localhost:8080'}, compilerOptions:{experimental:{async:true}}}), {
  name:'renderer-experiment-control',
  configureServer(server) {
   server.middlewares.use('/__experiment', async (req,res) => {
    const nodeEnv = process.env.NODE_ENV;
    try {
     process.env.NODE_ENV = 'production';
     const started = performance.now();
     if(req.url === '/full') {
      const builder = await createBuilder({configFile:resolve('vite.config.js'), logLevel:'error'});
      await builder.buildApp();
     } else if(req.url === '/update') {
      await globalThis.__rendererRebuild();
     } else if(req.url.startsWith('/fetch?')) {
      const args = JSON.parse(decodeURIComponent(req.url.slice(7)));
      const result = args.data.name === 'getBuiltins' ? [] : await globalThis.__rendererDev.environments.goja.fetchModule(...args.data.data);
      res.end(JSON.stringify({result})); return;
     } else if(req.url === '/runtime') {
      const plugin = {...globalThis.__rendererPlugin, apply:'serve'};
      globalThis.__rendererDev = await createServer({configFile:false,root:process.cwd(),logLevel:'error',server:{middlewareMode:true},plugins:[sveltekit({adapter:{name:'runtime-probe',vite:{plugins:{post:[plugin]}},adapt(){}},paths:{origin:'http://localhost:8080'},compilerOptions:{experimental:{async:true}}})]});
      const {rolldown} = await import('vite/rolldown');
      const bundle = await rolldown({input:resolve('runtime-entry.js'),platform:'neutral'});
      await bundle.write({file:resolve('runtime.js'),format:'cjs',dynamicImportInCjs:false,banner:readFileSync('POLYFILL','utf8')});
      await bundle.close();
     } else if(req.url === '/runner') {
      const {rolldown} = await import('vite/rolldown');
      const transformed = await server.environments.ssr.fetchModule('/src/routes/message.js');
      writeFileSync('runner-module.js', 'export default ' + JSON.stringify(transformed));
      const bundle = await rolldown({input:resolve('runner-entry.js'), platform:'neutral'});
      await bundle.write({file:resolve('runner.js'),format:'iife'});
      await bundle.write({file:resolve('runner-cjs.js'),format:'cjs',dynamicImportInCjs:false});
      await bundle.close();
     } else { res.end('ready'); return; }
     res.end(JSON.stringify({milliseconds:performance.now()-started}));
    } catch(error) { res.statusCode=500;res.end(error.stack); } finally { process.env.NODE_ENV = nodeEnv; }
   });
  }
 }]
};
