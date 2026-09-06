<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { getSession, signIn, signOut } from '../session.remote';

	const session = getSession();
	let name = $state('');
	let error = $state('');

	// The junkyard app used a `+page.server.ts` action here. skgo has no form
	// actions, so the same behaviour is hand-wired onto a Go command.
	async function login(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		const result = await signIn(name);
		if (result.error) {
			error = result.error;
			return;
		}
		await getSession().refresh();
		await goto('/');
	}

	async function logout(event: SubmitEvent) {
		event.preventDefault();
		await signOut();
		await getSession().refresh();
		await goto('/login?bye');
	}
</script>

<h1>Login</h1>

{#if (await session).user}
	<p data-testid="login-state">signed in as {(await session).user}</p>
	<form method="POST" onsubmit={logout}>
		<button data-testid="logout">Log out</button>
	</form>
{:else}
	<p data-testid="login-state">signed out</p>
	<form method="POST" onsubmit={login}>
		<label>
			Name
			<input name="name" data-testid="name" bind:value={name} />
		</label>
		<button data-testid="login">Log in</button>
	</form>
{/if}

{#if error}
	<p data-testid="login-error">{error}</p>
{/if}
{#if page.url.searchParams.has('bye')}
	<p data-testid="logged-out">you have been logged out</p>
{/if}
