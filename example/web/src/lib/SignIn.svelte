<script lang="ts">
	import { signIn, signOut, whoami } from './auth.remote';
	import { getTodos, watchCount } from '../routes/todos/todos.remote';

	let name = $state('');
	let busy = $state(false);

	const session = whoami();

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (name.trim() === '' || busy) return;
		busy = true;
		try {
			// The session cookie the command sets is already in force by the
			// time these queries run: they are resolved on the same request.
			await signIn(name).updates(whoami(), getTodos());
			// A live query is not a query: `updates` cannot carry it, and kit
			// caches it by (id, argument), so the open stream survives the new
			// session and keeps reporting the old visitor's count. `reconnect`
			// is the handle kit gives you for exactly this.
			await watchCount().reconnect();
			name = '';
		} finally {
			busy = false;
		}
	}

	async function leave() {
		busy = true;
		try {
			await signOut().updates(whoami(), getTodos());
			await watchCount().reconnect();
		} finally {
			busy = false;
		}
	}
</script>

{#if (await session).user}
	<span data-testid="session">Signed in as {(await session).user}</span>
	<button data-testid="sign-out" onclick={leave} disabled={busy}>Sign out</button>
{:else}
	<span data-testid="session">Signed out</span>
	<form onsubmit={submit}>
		<input data-testid="user" name="user" bind:value={name} placeholder="your name" />
		<button data-testid="sign-in" type="submit" disabled={busy}>Sign in</button>
	</form>
{/if}
