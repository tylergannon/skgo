---
title: Types and transport
description: How Go types become TypeScript types, and how a type with behaviour crosses the wire as a class.
division: skgo
section: Core concepts
order: 2080
---

Every value that crosses between Go and Svelte is declared once, in Go. `skgo generate` writes the TypeScript type for each struct a load returns, a remote function takes or returns, or an action answers, so the page is typed from the Go source and a change to a struct is a change to the page's types.

## From Go to TypeScript

A Go struct becomes a TypeScript type with one property per exported field, named by the field's JSON tag. The generated `types.ts` for a package lives beside it when the package is inside `src`, and under `src/lib/skgo/` otherwise; see [Generated files](/docs/skgo/project-structure#Generated-files). The projection follows the rules of [polytype](https://github.com/tylergannon/polytype), which SKGo uses to write the declarations:

| Go                                   | TypeScript                               |
| ------------------------------------ | ---------------------------------------- |
| `string`, `bool`                     | `string`, `boolean`                      |
| `int`, `int64`, `float64`, …         | `number`                                 |
| `[]T`                                | `Array<T>`                               |
| `map[string]T`                       | `Record<string, T>`                      |
| a struct                             | a named type in `types.ts`               |
| a named `string` or integer type     | its name; a union of literals when it has constants |
| `time.Time`                          | `string`, in RFC 3339                    |
| `polytype.Optional[T]`               | `T`, with the property optional          |
| `polytype.Nullable[T]`               | `T \| null`                              |
| `skgo.File`                          | `File`, in a form's argument             |
| `skgo.Deferred[T]`                   | `Promise<T>`, in a load's result only    |
| a type declared with `Transported`   | the class your `hooks.ts` builds         |

A type polytype cannot express fails `skgo generate` with a message saying why, rather than being written approximately. A `Deferred` anywhere but a load's result is one such case, because nothing else on the wire streams.

### Optional and nullable fields

A pointer or a zero value cannot tell "absent" from "set to zero" once it has been JSON. `polytype.Optional[T]` can: its `Present` field reports whether the value was there, and `omitzero` on the tag keeps an absent value out of the output.

```go example/web/src/routes/optional/optional.remote.go
```

## Transport

Kit's [`transport` hook](https://svelte.dev/docs/kit/hooks#Universal-hooks-transport) lets a custom type cross the wire as an instance of a class, with its methods, instead of a plain object. It has two halves, and in SKGo they sit next to each other at the top of `src`.

The Go half declares the type and the key it travels under:

```go example/web/src/hooks.go
```

The TypeScript half is Kit's own hook with the same key, declared `satisfies Transport`:

```ts example/web/src/hooks.ts
```

`Transport` comes from `src/transport.ts`, which `skgo generate` writes from `src/hooks.go`. It is Kit's own `Transport` type with each Go key added, as a `Transporter` from the class `hooks.ts` exports to the object Go sends for it:

```ts example/web/src/transport.ts
```

The payload is the type's JSON fields, so `decode`'s `data` is typed without an annotation and `encode` has to return what Go's decoder reads. Keys only the browser uses are still allowed, since Kit also passes snapshots and history state through the hook. `src/transport.ts` also checks the export: it does not type-check until `hooks.ts` exports a `transport` that agrees with Go, so a hook typed only with Kit's own `Transport` fails, and so does a missing one. `skgo check`, svelte-check and your editor report the error.

From then on a `businesslogic.Money` anywhere in a load's result, a remote function's answer or an action's data arrives in the browser as a `Money`, and the generated types name the class rather than describing its fields, so `price.format()` type-checks.

The same goes the other way. A command or form whose argument is a transported type receives the Go value, decoded from what the page's `encode` produced:

```go example/web/src/routes/(marketing)/pricing/pricing.remote.go
```

Go serialises a transported value as `["Money", …]` under the key both halves spell, which is Kit's wire format for the hook; during server rendering the renderer inside the Go process runs the page's own `decode`, so the document is rendered with the class's methods too. A key or a field that differs between the two halves is therefore a type error in `hooks.ts`, not a page that fails or renders `NaN`. A transported type cannot hold another one in its fields, because the codec writes the inner value as plain fields and no class would come back for it; `skgo generate` refuses it.

## Deferred values

A load's result may promise a value with `skgo.Deferred[T]`, which the page sees as `Promise<T>`; see [Streaming with promises](/docs/skgo/load#Streaming-with-promises). A transported type inside a deferred value crosses the same way once the value arrives.
