import type { PageLoad } from './$types';

export const load: PageLoad = ({ data, url }) => {
	const overdueOnly = url.searchParams.get('overdue') === '1';
	const invoices = data.invoices
		.map((invoice) => ({
			...invoice,
			overdue: invoice.outstandingCents > 0 && invoice.dueDate < data.asOf
		}))
		.filter((invoice) => !overdueOnly || invoice.overdue)
		.sort((a, b) => a.dueDate.localeCompare(b.dueDate));
	return {
		invoices,
		totalCents: invoices.reduce((total, invoice) => total + invoice.outstandingCents, 0),
		overdueOnly
	};
};
