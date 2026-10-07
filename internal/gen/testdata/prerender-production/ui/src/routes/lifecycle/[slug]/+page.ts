import { value } from './fixture.remote';
export function entries() { return [{slug:'alpha'}, {slug:'beta'}]; }
export async function load({ params, fetch }) { const response = await fetch('/lifecycle-api');
 await (await fetch(`/lifecycle-cookies/${params.slug}-set`)).text();
 await (await fetch(`/lifecycle-cookies/${params.slug}-delete`)).text();
 return { remote: await value(params.slug), universalFetched: 'Kit fetch: ' + await response.text() }; }
