<script lang="ts">
 import { onMount, tick } from 'svelte';
 import { refreshAll, invalidateAll, goto } from '$app/navigation';
 import { page } from '$app/state';
 import { order } from './orders.remote';
 let draft=$state(''); let count=$state(0); let status=$state('Ready');
 let component=$state(''); let documentID=$state('');
 const result=order();
 onMount(()=>{
  component=crypto.randomUUID();
  const w=window as any; documentID=w.__documentID ??= crypto.randomUUID();
  const handler=async(data:{mode:string,published:number})=>{
   if(data.mode==='reload'){location.reload();return;}
   try {if(data.mode==='invalidate')await invalidateAll();else await refreshAll();await tick();status=`${data.mode}: ${Date.now()-data.published}ms publication to visible`;}
   catch(error){status=String(error)}
  };
  import.meta.hot?.on('skgo:published',handler);
  return ()=>import.meta.hot?.off('skgo:published',handler);
 });
 async function update(value:string,mode:string){status='Compiling';const response=await fetch(`/__experiment/update?value=${value}&mode=${mode}`,{method:'POST'});if(!response.ok)status=await response.text();}
</script>
<h1>Native Go + Vite cooperation</h1>
<p id="frontend">FRONTEND_ONE</p>
<button onclick={()=>goto('', {state:{proof:'SHALLOW_NATIVE_41'},shallow:true,replace:true})}>Set shallow state</button><p id="shallow">Shallow: {JSON.stringify(page.state)}</p>
<label>Unsent draft <input id="draft" bind:value={draft}/></label>
<button onclick={()=>count++}>Increment</button><p id="count">Local count: {count}</p>
<svelte:boundary>
 <p id="total">Go total: {(await result).total}</p>
 {#snippet pending()}<p>Loading Go</p>{/snippet}
 {#snippet failed(error)}<p id="query-error">{String(error)}</p>{/snippet}
</svelte:boundary>
<button onclick={()=>update('2400','refresh')}>Patch 2400 + refreshAll</button>
<button onclick={()=>update('1800','invalidate')}>Patch 1800 + invalidateAll</button>
<button onclick={()=>update('1200','reload')}>Patch 1200 + document reload</button>
<button onclick={()=>update('broken','refresh')}>Compile error</button>
<button onclick={()=>update('900','refresh')}>Repair 900 + refreshAll</button>
<p id="status">{status}</p><p id="component">Component: {component}</p><p id="document">Document: {documentID}</p>
