---
title: "skgo: environment"
summary: "Environment and LoadEnvironment, typed environment variables declared in Go."
---

Kit's `$env` modules, declared in Go. A struct marked with `Environment`
names the variables the application needs and their types; `skgo generate`
writes the declaration the `skgo env` command checks and the typed accessors
the Svelte side imports. `LoadEnvironment` reads and validates the values at
startup and returns the snapshot the server keeps.

## Environment

```go
func Environment[T any]() struct{}
```

Environment declares the application's environment in src/env.go. T must be a struct with exported string, bool, int or float64 fields tagged `env:"NAME[,public][,static]"`. Pointer fields are optional. Strings retain their exact contents; booleans accept true/false and yes/no, case-insensitively. Integers use base ten and must fit both Go int and JavaScript's safe integer range. float64 values must be finite decimal numbers. Whitespace is not trimmed. Unset required fields fail at build and startup; an empty string is valid only for string fields. Static fields bind at build, dynamic fields at startup. This marker does not read values. Run skgo generate after editing it.

## LoadEnvironment

```go
func LoadEnvironment[T any](build fs.FS, lookup func(string) (string, bool)) (T, *EnvironmentSnapshot, error)
```

LoadEnvironment validates startup values using the same parser used at build time. Static values come exclusively from env.json in the embedded build. lookup is normally os.LookupEnv. Errors name variables but never print values.

## EnvironmentSnapshot

```go
type EnvironmentSnapshot struct {
	// contains filtered or unexported fields
}
```

EnvironmentSnapshot contains the values validated once for this application. Accessors return copies so callers cannot change the renderer's environment.

### DynamicPublic

```go
func (s *EnvironmentSnapshot) DynamicPublic() map[string]any
```

DynamicPublic returns public values whose binding time is startup.

### Private

```go
func (s *EnvironmentSnapshot) Private() map[string]any
```

Private returns every private value.

### Public

```go
func (s *EnvironmentSnapshot) Public() map[string]any
```

Public returns every public value, including static values.
