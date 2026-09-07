import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

// The price is read off the row for that plan rather than off the list as a
// whole, so a page that rendered one row correctly and the rest as `[object
// Object]` fails instead of passing on the first match.
Then(
	'the plan {string} costs {string}',
	async ({ page, shot }, plan: string, price: string) => {
		const row = page.getByTestId('plan').filter({ hasText: plan });
		await expect(row).toHaveCount(1);
		await expect(row).toHaveText(`${plan} — ${price}`);
		await shot();
	}
);

When('I ask Go about a price of {string}', async ({ page }, price: string) => {
	const button = page.getByTestId('ask');
	await expect(button).toHaveText(`Ask Go about ${price}`);
	await button.click();
});

Then('Go says it heard {string}', async ({ page, shot }, price: string) => {
	await expect(page.getByTestId('quote-heard')).toHaveText(price);
	await shot();
});

Then('Go says double that is {string}', async ({ page, shot }, price: string) => {
	await expect(page.getByTestId('quote-doubled')).toHaveText(price);
	await shot();
});

// The featured plan is a server load's, so it came down inside the document
// rather than in a request of its own. Nothing on the wire spells the price:
// Go sends 4500 cents, and the only thing that can write "$45.00" is the
// Money the boot script's `app.decode` built out of them.
Then('the featured plan costs {string}', async ({ page, shot }, price: string) => {
	await expect(page.getByTestId('featured')).toHaveText(`Startup — ${price}`, {
		timeout: 15_000
	});
	await shot();
});
