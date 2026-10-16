import http from 'node:http';
// This POC explicitly bridges only same-origin event.fetch('/go/...').
export async function handleFetch({event,request,fetch}) {
 const url=new URL(request.url);
 if(url.origin!==event.url.origin||!url.pathname.startsWith('/go/'))return fetch(request);
 const headers=Object.fromEntries(request.headers);
 if(request.credentials!=='omit')for(const key of ['cookie','authorization'])if(!headers[key]&&event.request.headers.has(key))headers[key]=event.request.headers.get(key);
 headers.host=url.host;
 const socketPath=event.request.headers.get('x-poc-wrong-target')==='1'?process.env.WRONG_GO_SOCK:process.env.GO_SOCK;
 const body=request.method==='GET'||request.method==='HEAD'?null:Buffer.from(await request.arrayBuffer());
 return new Promise((resolve,reject)=>{
  const req=http.request({socketPath,path:url.pathname+url.search,method:request.method,headers,agent:false},res=>{
   const chunks=[];res.on('data',x=>chunks.push(x));res.on('end',()=>{const h=new Headers();for(let i=0;i<res.rawHeaders.length;i+=2)h.append(res.rawHeaders[i],res.rawHeaders[i+1]);resolve(new Response(Buffer.concat(chunks),{status:res.statusCode,headers:h}))});res.on('error',reject)
  });req.on('error',reject);req.end(body)
 });
}
