import { Money } from "./hooks";
import type { Transport } from "./transport";

// The hook in src/hooks.ts already has to satisfy Transport; these are the
// drifts that must not. Each one is a page that would render `$NaN.NaN` or a
// 500 if TypeScript let it through.

const decodesTheWrongField = {
  Money: {
    encode: (value) => value instanceof Money && { cents: value.cents },
    // @ts-expect-error Go sends `cents`, not `amount`
    decode: (data) => new Money(data.amount),
  },
} satisfies Transport;
void decodesTheWrongField;

const encodesTheWrongField = {
  Money: {
    // @ts-expect-error Go's decoder reads `cents`
    encode: (value) => value instanceof Money && { amount: value.cents },
    decode: (data) => new Money(data.cents),
  },
} satisfies Transport;
void encodesTheWrongField;

const misspellsTheKey = {
  Mony: {
    encode: (value: unknown) => value instanceof Money && { cents: value.cents },
    decode: (data: { cents: number }) => new Money(data.cents),
  },
  // @ts-expect-error Go encodes a Money under `Money`, so this hook has no entry for it
} satisfies Transport;
void misspellsTheKey;

// Kit's own annotation widens the hook to `Record<string, Transporter>`, which
// says nothing about Go; it is not a Transport.
const widened: import("@sveltejs/kit/hooks").Transport = {};
// @ts-expect-error Money is missing from a hook typed only as kit's Transport
const unchecked: Transport = widened;
void unchecked;

// A key only the browser uses is kit's to have: snapshots and history state run
// through the same hook.
const browserOnlyKey = {
  Money: {
    encode: (value) => value instanceof Money && { cents: value.cents },
    decode: (data) => new Money(data.cents),
  },
  Point: {
    encode: (value: unknown) => Array.isArray(value) && value,
    decode: (data: number[]) => data,
  },
} satisfies Transport;
void browserOnlyKey;
