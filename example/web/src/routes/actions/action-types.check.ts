import type { ActionFailure } from '@sveltejs/kit';
import type { Money } from '../../hooks';
import type { actions } from './+page.server';

type Save = Awaited<ReturnType<typeof actions.save>>;
type Success = Exclude<Save, ActionFailure<unknown>>;
type Failure = Extract<Save, ActionFailure<unknown>>;

// Correct uses must compile through Kit's generated action export and the
// transport class returned by Go.
function accepted(success: Success, failure: Failure): [string, Money, string, string] {
	const price: Money = success.price;
	return [success.receipt, price, price.format(), failure.data.emailError];
}
void accepted;

// @ts-expect-error validation fields exist only in failure data
type SuccessEmailError = Success['emailError'];
// @ts-expect-error receipts exist only in success data
type FailureReceipt = Failure['data']['receipt'];
