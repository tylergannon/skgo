export const prerender = true;

export async function load({ fetch }) {
	const refused = await fetch('/hook-refused');
	const redirected = await fetch('/hook-redirected');
	return {
		refused: `${refused.status} ${await refused.text()}`,
		redirected: `${redirected.status}`
	};
}
