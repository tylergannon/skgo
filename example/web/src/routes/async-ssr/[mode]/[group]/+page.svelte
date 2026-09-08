<script lang="ts">
 import type { PageProps } from './$types';
 import { getValue, getBatch, watchValue, getSeed, getDependent, getHeld } from './probe.remote';
 let { params }: PageProps = $props();
</script>

<h1>Concurrent server rendering</h1>
<p>Three independent Go operations must start before any can finish.</p>
<p data-testid="probe-group">Request: {params.group}</p>

<svelte:boundary>
 {#if params.mode === 'abandoned' || params.mode === 'current'}
  <p data-testid="held-value">{await getHeld({group:params.group,key:params.mode})}</p>
 {:else if params.mode === 'dependent'}
  <p data-testid="dependent-value">{await getDependent(await getSeed(params.group))}</p>
 {:else if params.mode === 'mixed'}
  <ul data-testid="probe-values">
   <li>{await getValue({group:params.group,key:'amber'})}</li>
   <li>{await getBatch({group:params.group,key:'one'})}</li>
   <li>{await getBatch({group:params.group,key:'two'})}</li>
   <li>{await watchValue({group:params.group,key:'live'})}</li>
  </ul>
 {:else}
  <ul data-testid="probe-values">
   <li>{await getValue({group:params.group,key:'amber'})}</li>
   <li>{await getValue({group:params.group,key:'birch'})}</li>
   <li>{await getValue({group:params.group,key:'cobalt'})}</li>
  </ul>
 {/if}
 {#snippet failed(error)}
  <p data-testid="probe-failure">{(error as Error).message}</p>
 {/snippet}
</svelte:boundary>
