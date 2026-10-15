<script>
import {onMount,tick} from 'svelte';
import {refreshAll} from '$app/navigation';
let {data}=$props();
let draft=$state('');let count=$state(0);let hydrated=$state(false);let error=$state('');let elapsed=$state(0);
onMount(()=>{hydrated=true;});
async function update(value){const start=performance.now();const response=await fetch('/__control/value?value='+value);if(!response.ok){error=await response.text();return;}await refreshAll();await tick();elapsed=performance.now()-start;error='';}
if(import.meta.hot)import.meta.hot.on('cooperation:refresh',async()=>{await refreshAll();});
</script>
<h1>COOPERATION_FRONTEND_ONE</h1>
<p id="value">Go load: {data.value}</p><p id="hydrated">Hydrated: {String(hydrated)}</p>
<label>Draft <input bind:value={draft}/></label><button onclick={()=>count++}>Count {count}</button>
<button onclick={()=>update('2400')}>Go value 2400</button><button onclick={()=>update('broken')}>Break Go load</button><button onclick={()=>update('900')}>Repair Go load</button>
<button onclick={async()=>{await fetch('/__control/frontend');}}>Edit Svelte</button>
<p>Update ms: {elapsed.toFixed(2)}</p><p id="error">{error}</p>
