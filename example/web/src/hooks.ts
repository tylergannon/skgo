import type { Transport } from "./transport";

/**
 * Money is the browser's half of the domain type. It is a class, not an
 * interface, because the point of the `transport` hook is that an instance
 * arrives with its behaviour: a page calls `price.format()`.
 *
 * The Go half is `businesslogic.Money`, declared in `src/hooks.go`.
 */
export class Money {
  readonly cents: number;

  constructor(cents: number) {
    this.cents = cents;
  }

  format(): string {
    const sign = this.cents < 0 ? "-" : "";
    const cents = Math.abs(this.cents);
    return `${sign}$${Math.floor(cents / 100)}.${String(cents % 100).padStart(2, "0")}`;
  }
}

/**
 * transport is kit's universal hook for custom types. Each key is the tag the
 * value travels under, and it has to be spelled the same in `src/hooks.go`:
 * Go serializes a Money as `["Money", …]`, and this is what reads it back.
 *
 * `encode` returns `false` for anything that is not a Money, which is how kit
 * asks "is this yours?". What it returns for a match is what the Go decoder
 * receives when a Money travels the other way, in a remote function's argument.
 *
 * `satisfies Transport` checks all of that against Go: `skgo generate` writes
 * `src/transport.ts` from `src/hooks.go`, so a key or field that drifts on
 * either side is a type error here rather than a broken page.
 */
export const transport = {
  Money: {
    encode: (value) => value instanceof Money && { cents: value.cents },
    decode: (data) => new Money(data.cents),
  },
} satisfies Transport;

// Kit owns invocation and client caching; Go owns the universal hook body.
export { reroute } from "@skgo/sveltekit-adapter/reroute";
