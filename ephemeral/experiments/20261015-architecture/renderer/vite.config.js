import { sveltekit } from '@sveltejs/kit/vite';
import { createBuilder, createServer, DevEnvironment } from 'vite';
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
 cacheDir:resolve('.vite-cache'),
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
      const environment = globalThis.__rendererDev.environments.goja;
      let result;
      if (args.data.name === 'getBuiltins') result = [];
      else {
       const values = [...args.data.data];
       // Use Vite's resolver at the transport boundary for bare imports added
       // by lowering after import-analysis. No resolver/graph is implemented here.
       if (values[1] && !values[0].startsWith('.') && !values[0].startsWith('/') && !values[0].startsWith('file:')) {
        const resolved = await environment.pluginContainer.resolveId(values[0], values[1]);
        console.log('RESOLVE', values[0], JSON.stringify(resolved));
        if (resolved && !resolved.external) { values[0] = resolved.id; values[1] = undefined; }
       }
       result = await environment.fetchModule(...values);
       console.log('MODULE', values[0], result.externalize ? 'external:'+result.externalize : result.code ? result.code.length : 'cache');
      }
      res.end(JSON.stringify({result})); return;
     } else if(req.url === '/runtime') {
      globalThis.__rendererEvents = [];
      const plugin = {...globalThis.__rendererPlugin, apply:'serve', config() {
       const config = globalThis.__rendererPlugin.config();
       config.environments.goja.dev = { createEnvironment(name, resolved) {
        return new DevEnvironment(name, resolved, {hot:true,transport:{skipFsCheck:true,
         send(payload) { console.log('HOT',JSON.stringify(payload)); globalThis.__rendererEvents.push(payload); },
         on() {}, off() {}
        }});
       }};
       return config;
      }};
      globalThis.__rendererDev = await createServer({configFile:false,root:process.cwd(),cacheDir:resolve('.vite-runtime-cache'),logLevel:'error',server:{middlewareMode:true},plugins:[sveltekit({adapter:{name:'runtime-probe',vite:{plugins:{post:[plugin]}},adapt(){}},paths:{origin:'http://localhost:8080'},compilerOptions:{experimental:{async:true}}})]});
      globalThis.__rendererSeen = {}; globalThis.__rendererDev.watcher.on('change', file => {globalThis.__rendererSeen[file] = (globalThis.__rendererSeen[file] || 0) + 1;});
      const {rolldown} = await import('vite/rolldown');
      const bundle = await rolldown({input:resolve('runtime-entry.js'),platform:'neutral'});
      await bundle.write({file:resolve('runtime.js'),format:'cjs',dynamicImportInCjs:false,banner:readFileSync('POLYFILL','utf8')});
      await bundle.close();
      const retained = await rolldown({input:resolve('retained-entry.js'),platform:'neutral'});
      await retained.write({file:resolve('retained.js'),format:'cjs',dynamicImportInCjs:false,banner:readFileSync('POLYFILL','utf8')});
      await retained.close();
     } else if(req.url.startsWith('/events?')) {
      const cursor = Number(req.url.slice(8));
      res.end(JSON.stringify({events:globalThis.__rendererEvents.slice(cursor),cursor:globalThis.__rendererEvents.length}));return;
     } else if(req.url === '/resources') {res.end(JSON.stringify(process.memoryUsage()));return;
     } else if(req.url === '/seen') {
      res.end(JSON.stringify(globalThis.__rendererSeen || {})); return;
     } else { res.end('ready'); return; }
     res.end(JSON.stringify({milliseconds:performance.now()-started}));
    } catch(error) { res.statusCode=500;res.end(error.stack); } finally { process.env.NODE_ENV = nodeEnv; }
   });
  }
 }]
};
