<script>
import {onMount,tick} from 'svelte';import {refreshAll} from '$app/navigation';import {order} from './orders.remote';
let {data}=$props();let draft=$state('');let count=$state(0);let hydrated=$state(false);let documentID=$state('');let notices=$state(0);let status=$state('Ready');const result=order();
onMount(()=>{hydrated=true;documentID=window.__listenerDocument??=crypto.randomUUID();const fn=async(data)=>{notices++;if(data.error){status=data.error;return;}await refreshAll();await tick();status='Published '+data.instance;};import.meta.hot?.on('skgo:published',fn);return()=>import.meta.hot?.off('skgo:published',fn);});
</script>
<h1>INSTANCE_LABEL FRONTEND_ONE</h1><p id="hydrated">Hydrated: {String(hydrated)}</p><label>Draft <input id="draft" bind:value={draft}/></label><button onclick={()=>count++}>Increment</button><p id="count">Count: {count}</p>
<svelte:boundary><p id="total">Go total: {(await result).total}</p>{#snippet pending()}<p>Loading Go</p>{/snippet}{#snippet failed(error)}<p>{String(error)}</p>{/snippet}</svelte:boundary>
<p id="ssr-value">Go SSR total: {data.value.total}</p><p id="notices">Notices: {notices}</p><p id="status">{status}</p><p id="document">Document: {documentID}</p>
