import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
export default defineConfig({
 cacheDir: '.vite-experiment',
 server: { host: '127.0.0.1', port: 56141, strictPort: true, hmr: { clientPort: 56142 } },
 plugins: [sveltekit({paths:{origin:'http://127.0.0.1:56142'},experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}}), {
  name:'experiment-go-publication',
  configureServer(server) {
   server.middlewares.use('/__published', (req,res) => {
    const url=new URL(req.url!, 'http://localhost');
    server.hot.send({type:'custom',event:'skgo:published',data:{mode:url.searchParams.get('mode'),published:Number(url.searchParams.get('published'))}});
    res.end('sent');
   });
  }
 }]
});
