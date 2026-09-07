<script lang="ts">
	let { data } = $props();
</script>

<h1 data-testid="title">Stream</h1>
<p data-testid="headline">{data.headline}</p>

<section>
	<h2>Ticker</h2>
	{#await data.ticker}
		<p data-testid="ticker-pending">waiting for the ticker…</p>
	{:then ticker}
		<p data-testid="ticker">{ticker}</p>
	{/await}
</section>

<section>
	<h2>Digest</h2>
	{#await data.digest}
		<p data-testid="digest-pending">waiting for the digest…</p>
	{:then digest}
		<ul data-testid="digest">
			{#each digest as line}
				<li data-testid="digest-line">{line}</li>
			{/each}
		</ul>
	{/await}
</section>

<section>
	<h2>{data.weather.heading}</h2>
	{#await data.weather.forecast}
		<p data-testid="forecast-pending">waiting for the forecast…</p>
	{:then forecast}
		<p data-testid="forecast">{forecast}</p>
	{/await}
</section>
