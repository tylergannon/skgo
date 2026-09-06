import type { Transport } from '@sveltejs/kit/hooks';

import { Money } from '#lib/money';

// `transport` is a universal hook: kit uses it to encode and decode custom
// classes across the wire, including remote-function payloads.
export const transport: Transport = {
	Money: {
		encode: (value) => value instanceof Money && [value.cents, value.currency],
		decode: ([cents, currency]: [number, string]) => new Money(cents, currency)
	}
};
