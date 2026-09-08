package skgo

import (
	"context"
	"encoding/json"
	"fmt"
)

// Registration doubles for the tests in this package.
//
// In a real app every registration is generated: `skgo generate` emits one
// concrete closure per remote function, whose decoder and encoder are the pair
// polytype's devalue backend generated for that function's own argument and
// result types. There is no way to write one by hand here, and no reason to
// want one — the closure's whole point is that the types are known at generate
// time.
//
// The tests in this package are about dispatch: routing, methods, cookies,
// events, refresh keys, panics, redirects, envelopes, streams. They need a
// registration, not a codec, so these helpers build one over an
// encoding/json round trip. That is deliberately *not* what production does,
// and the difference is exactly the one this package no longer has: a JSON
// round trip zero-fills a missing field where a generated decoder answers 400
// naming the path.
//
// The generated decoders are proved where they are generated: example/ runs
// the real ones, over the real wire, and example/wire_test.go pins what they
// accept and refuse.

// doubleDecode is the codec double: kit's argument tree into a Go value, with
// encoding/json. See the file comment for why it is not the production path.
func doubleDecode[In any](call Call) (In, error) {
	var in In
	if !call.Present {
		return in, nil
	}
	raw, err := json.Marshal(call.Arg)
	if err != nil {
		return in, Errorf(400, "Bad Request")
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, Errorf(400, "Bad Request")
	}
	return in, nil
}

// doubleEncode is the result half. It is the transport walk, which is what a
// generated closure uses for a result that can reach a transported type.
func doubleEncode(call Call, v any) (any, error) { return call.Transported(v) }

func doubleArgDecoder[In any]() func(any) (any, error) {
	return func(arg any) (any, error) { return doubleDecode[In](Call{Arg: arg, Present: true}) }
}

func doubleCall[In, Out any](fn func(context.Context, In) (Out, error)) RemoteFunc {
	return func(ctx context.Context, call Call) (any, error) {
		in, err := doubleDecode[In](call)
		if err != nil {
			return nil, err
		}
		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}
		return doubleEncode(call, out)
	}
}

func doubleCallNoArg[Out any](fn func(context.Context) (Out, error)) RemoteFunc {
	return func(ctx context.Context, call Call) (any, error) {
		if err := RefuseArgument(call); err != nil {
			return nil, err
		}
		out, err := fn(ctx)
		if err != nil {
			return nil, err
		}
		return doubleEncode(call, out)
	}
}

func NewQuery[In, Out any](module, name string, fn func(context.Context, In) (Out, error)) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindQuery, Module: module, Name: name, Fn: fn,
		Call: doubleCall(fn), DecodeArg: doubleArgDecoder[In](),
	})
}

func NewQueryNoArg[Out any](module, name string, fn func(context.Context) (Out, error)) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindQuery, Module: module, Name: name, Fn: fn, Call: doubleCallNoArg(fn),
	})
}

func NewCommand[In, Out any](module, name string, fn func(context.Context, In) (Out, error)) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindCommand, Module: module, Name: name, Fn: fn,
		Call: doubleCall(fn), DecodeArg: doubleArgDecoder[In](),
	})
}

func NewCommandNoArg[Out any](module, name string, fn func(context.Context) (Out, error)) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindCommand, Module: module, Name: name, Fn: fn, Call: doubleCallNoArg(fn),
	})
}

func NewForm[In, Out any](module, name string, fn func(context.Context, In) (Out, error)) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindForm, Module: module, Name: name, Fn: fn,
		Call: func(ctx context.Context, call Call) (any, error) {
			var in In
			if call.Present {
				if err := DecodeForm(call.Arg, &in); err != nil {
					return nil, err
				}
			}
			out, err := fn(ctx, in)
			if err != nil {
				return nil, err
			}
			return doubleEncode(call, out)
		},
	})
}

func NewLiveQuery[In, Out any](module, name string, fn func(context.Context, In, func(Out) error) error) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindLive, Module: module, Name: name, Fn: fn,
		DecodeArg: doubleArgDecoder[In](),
		Live: func(ctx context.Context, call Call, yield func(any) error) error {
			in, err := doubleDecode[In](call)
			if err != nil {
				return err
			}
			return fn(ctx, in, func(out Out) error {
				tree, err := doubleEncode(call, out)
				if err != nil {
					return err
				}
				return yield(tree)
			})
		},
	})
}

func NewLiveQueryNoArg[Out any](module, name string, fn func(context.Context, func(Out) error) error) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindLive, Module: module, Name: name, Fn: fn,
		Live: func(ctx context.Context, call Call, yield func(any) error) error {
			if err := RefuseArgument(call); err != nil {
				return err
			}
			return fn(ctx, func(out Out) error {
				tree, err := doubleEncode(call, out)
				if err != nil {
					return err
				}
				return yield(tree)
			})
		},
	})
}

func NewBatchQuery[In, Out any](module, name string, fn func(context.Context, []In) ([]Out, error)) *Remote {
	return NewRemote(RemoteSpec{
		Kind: KindBatch, Module: module, Name: name, Fn: fn,
		DecodeArg: doubleArgDecoder[In](),
		Batch: func(ctx context.Context, calls []Call) ([]any, error) {
			in := make([]In, len(calls))
			for i, call := range calls {
				decoded, err := doubleDecode[In](call)
				if err != nil {
					return nil, err
				}
				in[i] = decoded
			}
			outs, err := fn(ctx, in)
			if err != nil {
				return nil, err
			}
			if len(outs) != len(in) {
				return nil, fmt.Errorf("skgo: batch query %s was given %d arguments and answered %d results", name, len(in), len(outs))
			}
			res := make([]any, len(outs))
			for i, out := range outs {
				tree, err := doubleEncode(calls[i], out)
				if err != nil {
					return nil, err
				}
				res[i] = tree
			}
			return res, nil
		},
	})
}

func NewLoad[Out any](module string, fn func(context.Context) (Out, error)) *ServerLoad {
	return NewServerLoad(LoadSpec{Module: module, Run: func(ctx context.Context) (any, error) {
		out, err := fn(ctx)
		if err != nil {
			return nil, err
		}
		return out, nil
	}})
}
