import { sveltekit } from '@sveltejs/kit/vite';
import { createBuilder } from 'vite';
import { resolve } from 'node:path';
import { gojaEnvironment, nodeTable } from 'ADAPTER_ENV';
const goja=gojaEnvironment();
const generated={nodes:nodeTable(process.cwd(),[{component:'src/routes/+layout.svelte'},{component:'src/routes/+error.svelte'},{component:'src/routes/+page.svelte'}]),hooks:'export const transport = {};'};
const adapter={name:'cooperation-experiment',vite:{plugins:{post:[goja.plugin]}},async adapt(){await goja.build(generated,'bundle.js');globalThis.__cooperationRebuild=()=>goja.build(generated,'bundle.js');}};
const appRoute='^/(?:$|__data\\.json(?:\\?|$)|api(?:[/?]|$)|_app/remote/|static-marker\\.txt(?:\\?|$))';
export default {
 cacheDir:'.vite-cooperation',
 server:{host:'127.0.0.1',port:56311,strictPort:true,fs:{allow:['WORK_ROOT']},proxy:{[appRoute]:{target:'http://127.0.0.1:56312',changeOrigin:false}}},
 plugins:[sveltekit({adapter,paths:{origin:'http://127.0.0.1:56313'},experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}}),{
 name:'cooperation-control',configureServer(server){
 server.middlewares.use('/__cooperation',async(req,res)=>{try{
 if(req.url==='/build'){const old=process.env.NODE_ENV;process.env.NODE_ENV='production';try{const builder=await createBuilder({configFile:resolve('vite.config.js'),logLevel:'error'});await builder.buildApp();}finally{process.env.NODE_ENV=old;}res.end('built');}
 else if(req.url==='/rebuild'){const t=performance.now();await globalThis.__cooperationRebuild();res.end(JSON.stringify({milliseconds:performance.now()-t}));}
 else if(req.url==='/notify'){server.hot.send({type:'custom',event:'cooperation:refresh',data:{}});res.end('sent');}
 else res.end('ready');
 }catch(error){res.statusCode=500;res.end(error.stack)}});
 }}]
};
