<script lang="ts">
	import { getPlans } from './pricing.remote';
</script>

<h1 data-testid="title">Pricing</h1>

<svelte:boundary>
	<ul data-testid="plans">
		{#each await getPlans() as plan (plan.name)}
			<li data-testid="plan">{plan.name} — ${plan.price}</li>
		{/each}
	</ul>
	{#snippet pending()}
		<p data-testid="plans-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="plans-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
