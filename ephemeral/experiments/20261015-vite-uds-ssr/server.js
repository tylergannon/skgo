// Small supported Vite/Kit middleware host. No Vite patch or alternate SSR renderer.
import http from 'node:http';
import {writeFileSync} from 'node:fs';
import {createServer} from 'vite-plus';
let vite;
const server=http.createServer((req,res)=>vite.middlewares(req,res));
vite=await createServer({server:{middlewareMode:{server},ws:{server,path:'/__hmr',host:'127.0.0.1',clientPort:Number(new URL(process.env.PUBLIC_ORIGIN).port),protocol:'ws'}}});
server.listen(process.env.VITE_SOCK,()=>writeFileSync(process.env.READY_FILE,JSON.stringify({socket:process.env.VITE_SOCK,httpServerIsNull:vite.httpServer===null,pid:process.pid})));
async function stop(){await vite.close();server.close(()=>process.exit(0));setTimeout(()=>process.exit(0),1000).unref()}
process.on('SIGTERM',stop);process.on('SIGINT',stop);
