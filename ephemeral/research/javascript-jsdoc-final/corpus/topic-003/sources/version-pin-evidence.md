# Version pinning evidence: reference/polytype is rc.9, skgo requires v1.1.0

- Origin: `/Users/tyler/.codex/worktrees/ef7a/skgo/go.mod`
- Version: mixed -- see below
- Retrieval: retrieved 2026-09-26
- Citations below use the origin file's own line numbers (shown as `L<n>`).

---

## A. skgo go.mod L1-L12 (pinned requirement)

```go
module github.com/tylergannon/skgo

go 1.27.1

require (
	github.com/dop251/goja v0.0.0-20260911104922-fabc3b8078ad
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14
	github.com/tylergannon/polytype v1.1.0
	golang.org/x/mod v0.41.0
	golang.org/x/term v0.46.0
	golang.org/x/tools v0.50.0
)
```

## B. skgo go.sum L29-L30

```
github.com/tylergannon/polytype v1.1.0 h1:tKkhW5b0yb8wFNAWYPSPY5WawZRvz3ou1HDxb2DLo4A=
github.com/tylergannon/polytype v1.1.0/go.mod h1:Y35uUigmOAa0TGHwEWxkGqg8YHm7Xs4SxS+tOWF2QKQ=
```

## C. `readlink /Users/tyler/src/skgo/ephemeral/inspiration/reference/polytype`

```
/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.0.0-rc.9
```

## D. reference/polytype/go.mod L1-L11 (module file carries no version of itself)

```go
module github.com/tylergannon/polytype

go 1.27

require (
	github.com/dave/dst v0.27.3
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	github.com/stretchr/testify v1.10.0
	github.com/tylergannon/structtag v0.1.0
	golang.org/x/tools v0.49.0
)
```

## E. Go module cache .info (authoritative version -> commit metadata)

```
v1.1.0:     {"Version":"v1.1.0","Time":"2026-09-18T23:36:30Z","Origin":{"VCS":"git","Hash":"3d70c98617bdb6edea31abc50faec869a42c37a5"}}
v1.0.0-rc.9: {"Version":"v1.0.0-rc.9","Time":"2026-09-06T03:27:53Z","Origin":{"VCS":"git","Hash":"7995de4663adb172f770d48925934dc6707b6dfe"}}
```

## F. Diff summary: reference (rc.9) vs pinned v1.1.0 for the three assigned files

```
declare.go       DIFFERS substantially (marker-only struct{} vs spec ConfigurationSpec; FieldRef params)
doc_generate.go  IDENTICAL
declare_test.go  DIFFERS only in Field[...] call syntax
go.mod           DIFFERS (v1.1.0 adds github.com/dop251/goja require + 3 indirect deps)
```
(command: `diff <reference>/declare.go <v1.1.0>/declare.go` etc., run 2026-09-26)
