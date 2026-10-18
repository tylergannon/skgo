import { defineParams } from "@sveltejs/kit/params";

export const params = defineParams({
  Numeric: (value: string) => value,
  Aliased: (value: string) => value,
  Other: (value: string) => value,
  Flag: (value: string) => value,
  Blank: (value: string) => value,
  MaybeRef: (value: string) => value,
  Pointer: (value: string) => value,
  Generic: (value: string) => value,
  Structure: (value: string) => value,
  Function: (value: string) => value,
  FunctionAlias: (value: string) => value,
  Slice: (value: string) => value,
  Map: (value: string) => value,
  Channel: (value: string) => value,
  PublicHidden: (value: string) => value,
  StructOrder: (value: string) => value,
  LegacyOrder: (value: string) => value,
  Interface: (value: string) => value,
  Array: (value: string) => value,
});
