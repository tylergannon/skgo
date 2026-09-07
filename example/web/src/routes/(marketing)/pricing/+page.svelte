<script lang="ts">
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

<!-- format() is a method on the class in src/hooks.ts, and it is called while
     this page is rendered. Go sends 4500 cents under the transport key; the
     engine has the app's own decoders, so what reaches this line is a Money and
     the price is already in the document. A plain object would throw here and
     the visitor would get the shell instead. -->
<p data-testid="featured">{data.featured.name} — {data.featured.price.format()}</p>

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
