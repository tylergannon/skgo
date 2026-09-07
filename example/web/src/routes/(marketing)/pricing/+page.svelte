<script lang="ts">
	import { browser } from '$app/env';
	import { Money } from '../../../hooks';
	import { getPlans, quoteFor } from './pricing.remote';

	// The featured plan comes from the Go load, so it travelled down inside the
	// document rather than in a request of its own.
	let { data } = $props();

	// What Go said about a price this page sent it. Empty until the button is
	// pressed, so nothing here is on screen unless a Money made the round trip.
	let heard = $state('');
	let doubled = $state('');

	async function askAboutTeam() {
		// A real Money instance, built here in the browser. kit's transport
		// encodes it and Go's decoder turns it back into a businesslogic.Money.
		const quote = await quoteFor(new Money(2000));
		heard = quote.heard;
		doubled = quote.doubled;
	}
</script>

<h1 data-testid="title">Pricing</h1>

<!-- format() is the browser's half of the transport hook: the class lives in
     src/hooks.ts, so the price can only be written once kit's client has turned
     what the document carried back into a Money. `browser` is what says that
     moment has come; on the server there is no Money to ask. -->
{#if browser}
	<p data-testid="featured">{data.featured.name} — {data.featured.price.format()}</p>
{/if}

<svelte:boundary>
	<ul data-testid="plans">
		{#each await getPlans() as plan (plan.name)}
			<!-- format() is a method. A plain object would throw here, which is
			     exactly what happened before the transport hook existed. -->
			<li data-testid="plan">{plan.name} — {plan.price.format()}</li>
		{/each}
	</ul>
	{#snippet pending()}
		<p data-testid="plans-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="plans-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<button data-testid="ask" onclick={askAboutTeam}>Ask Go about $20.00</button>

{#if heard}
	<p data-testid="quote-heard">{heard}</p>
	<p data-testid="quote-doubled">{doubled}</p>
{/if}
