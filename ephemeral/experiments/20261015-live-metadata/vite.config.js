import {sveltekit} from '@sveltejs/kit/vite';
import {createBuilder} from 'vite';
import {resolve,join} from 'node:path';
import {readFileSync,writeFileSync,mkdirSync,realpathSync} from 'node:fs';
import {pathToFileURL} from 'node:url';
import {gojaEnvironment,nodeTable} from 'ADAPTER_ENV';
const root=process.cwd();const kitRoot=realpathSync(join(root,'node_modules/@sveltejs/kit'));
const {default:create_manifest_data}=await import(pathToFileURL(join(kitRoot,'src/core/sync/create_manifest_data/index.js')).href);
const sync=await import(pathToFileURL(join(kitRoot,'src/core/sync/sync.js')).href);
const goja=gojaEnvironment();
let liveConfig;let generation=0;
const adapter={name:'live-metadata',vite({config}){liveConfig=config;return {plugins:{post:[goja.plugin]}}},async adapt(builder){const data=create_manifest_data(builder.config,root);await goja.build({nodes:nodeTable(root,data.nodes),hooks:'export const transport={};'},'bootstrap-bundle.js');globalThis.__liveGojaBuild=(data,outfile)=>goja.build({nodes:nodeTable(root,data.nodes),hooks:'export const transport={};'},outfile);}};
export default {cacheDir:'.vite-live',server:{host:'127.0.0.1',port:56712,strictPort:true,ws:{clientPort:56711}},plugins:[sveltekit({adapter,paths:{origin:'http://127.0.0.1:56711'},experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}}),{name:'live-metadata-tools',configureServer(server){server.middlewares.use('/__live',async(req,res)=>{try{if(req.url==='/bootstrap'){const old=process.env.NODE_ENV;process.env.NODE_ENV='production';try{const b=await createBuilder({configFile:resolve('vite.config.js'),logLevel:'error'});await b.buildApp()}finally{process.env.NODE_ENV=old};res.end('built');return;}
 if(req.url==='/publish'){server.environments.client.moduleGraph.invalidateAll();server.hot.send({type:'full-reload'});res.end('published');return;}
 if(req.url==='/snapshot'){
  const data=create_manifest_data(liveConfig,root);sync.create(liveConfig,root,data,false);
  const output=`generations/${++generation}`;mkdirSync(output,{recursive:true});await globalThis.__liveGojaBuild(data,`${output}/bundle.js`);
  writeFileSync(`${output}/app.html`,readFileSync('src/app.html','utf8').replace('%sveltekit.head%','%sveltekit.head%<script type="module" src="/@vite/client"></script>'));
  writeFileSync(`${output}/error.html`,'<h1>%sveltekit.status% %sveltekit.error.message%</h1>');writeFileSync(`${output}/index.html`,'SPA_ONLY');mkdirSync(`${output}/client`,{recursive:true});
  const manifest={remotes:[],actions:[],appDir:liveConfig.appDir,base:liveConfig.paths.base,nodes:data.nodes.map(n=>n.server||''),loads:data.nodes.filter(n=>n.server).map(n=>n.server),routes:data.routes.filter(r=>r.page).map(r=>({id:r.id,pattern:r.pattern.source,params:r.params,page:{layouts:Array.from(r.page.layouts,n=>n??-1),errors:Array.from(r.page.errors,n=>n??-1),leaf:r.page.leaf}})),ssr:{bundle:'bundle.js',template:'app.html',errorTemplate:'error.html',target:'es2022',globalName:'__sveltekit_dev',client:{start:`/@fs${kitRoot}/src/runtime/client/entry.js`,app:`/@fs${root}/.svelte-kit/generated/dev/client/app.js`},nodes:data.nodes.map((n,i)=>({index:i,component:!!n.component,ssr:n.page_options?.ssr??null,csr:n.page_options?.csr??null}))}};
  const seen=new WeakSet();const raw=JSON.stringify(data,(k,v)=>{if(v instanceof RegExp)return {source:v.source,flags:v.flags};if(v&&typeof v==='object'){if(seen.has(v))return '[Repeated reference]';seen.add(v)}return v},2);writeFileSync(`${output}/kit-raw.json`,raw);writeFileSync(`${output}/skgo.manifest.json`,JSON.stringify(manifest,null,2));writeFileSync(`${output}/client-app.js`,readFileSync('.svelte-kit/generated/dev/client/app.js'));for(let i=0;i<data.nodes.length;i++)writeFileSync(`${output}/client-node-${i}.js`,readFileSync(`.svelte-kit/generated/dev/client/nodes/${i}.js`));
  res.setHeader('Content-Type','application/json');res.end(JSON.stringify({generation,output,manifest,kitVersion:JSON.parse(readFileSync(join(kitRoot,'package.json'))).version}));return;
 }
 res.end('ready');}catch(e){res.statusCode=500;res.end(e.stack)}})}}]};
