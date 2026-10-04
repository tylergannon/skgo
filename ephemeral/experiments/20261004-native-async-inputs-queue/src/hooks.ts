import { Money } from './lib/money';
export const transport = { Money: { encode: (value:unknown) => value instanceof Money && [value.cents], decode: ([cents]:number[]) => new Money(cents) } };