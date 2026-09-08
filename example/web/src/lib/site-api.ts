import { parse } from 'yaml';
import { getSite as getSiteQuery, type Site } from '../routes/site.remote';

// This ordinary application module is deliberately between the component and
// the generated remote. The schema is parsed at module evaluation time so the
// fixture also proves that dependencies with Node/default conditional exports
// choose an implementation the embedded engine can execute.
const schema = parse(`
required:
  - name
  - colocated
`) as { required: Array<keyof Site> };

export async function getSite(): Promise<Site> {
	const site = await getSiteQuery();
	for (const field of schema.required) {
		if (!site[field]) throw new Error(`site remote omitted ${field}`);
	}
	return site;
}
