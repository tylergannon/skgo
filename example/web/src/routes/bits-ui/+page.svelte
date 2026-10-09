<script lang="ts">
	import { Dialog, ScrollArea } from 'bits-ui';
	let open = $state(false);
</script>

<h1>Bits UI server rendering</h1>
<Dialog.Root bind:open>
	<Dialog.Trigger>Open library dialog</Dialog.Trigger>
	<Dialog.Content>
		<Dialog.Title>Library dialog</Dialog.Title>
		<Dialog.Description>Hydrated Bits UI dialog content.</Dialog.Description>
		<Dialog.Close>Close library dialog</Dialog.Close>
	</Dialog.Content>
</Dialog.Root>
<p data-testid="dialog-state">{open ? 'open' : 'closed'}</p>

<ScrollArea.Root type="always" style="height: 180px; width: 320px; overflow: hidden;">
	<ScrollArea.Viewport data-testid="library-scroll-viewport" style="height: 100%; width: 100%;">
		{#each Array.from({ length: 20 }, (_, i) => i + 1) as number}
			<p>Library row {number}</p>
		{/each}
	</ScrollArea.Viewport>
	<ScrollArea.Scrollbar orientation="vertical" style="width: 10px; background: #ddd;">
		<ScrollArea.Thumb style="background: #555; border-radius: 5px;" />
	</ScrollArea.Scrollbar>
</ScrollArea.Root>
