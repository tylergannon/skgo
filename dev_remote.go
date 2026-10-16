package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tylergannon/skgo/internal/remotearg"
)

// Native callbacks expose Go values, never browser remote HTTP envelopes.
// Kit retains batching, live iterators and form submission/cache ownership.
func (d *NativeDevelopment) remote(ctx context.Context, raw []byte, w http.ResponseWriter) error {
	var input struct {
		Module   string   `json:"module"`
		Name     string   `json:"name"`
		Payload  string   `json:"payload"`
		Payloads []string `json:"payloads"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	rs := d.renderer.remotes
	var fn *Remote
	for _, candidate := range rs.fns {
		if candidate.module == input.Module && candidate.name == input.Name {
			fn = candidate
			break
		}
	}
	if fn == nil {
		return fmt.Errorf("skgo: unknown native remote %s/%s", input.Module, input.Name)
	}
	request, _ := ctx.Value(prerenderRequestKey{}).(*http.Request)
	if request == nil {
		return fmt.Errorf("skgo: missing native request owner")
	}
	ev := rs.newEvent(request, fn.kind == KindForm || fn.kind == KindCommand)
	ctx = withEvent(ctx, ev)
	answer := func(value any, err error) error {
		if err != nil {
			if redirect := asRedirect(err); redirect != nil {
				return json.NewEncoder(w).Encode(map[string]any{"type": "redirect", "redirect": map[string]any{"status": redirect.status(), "location": redirect.Location}})
			}
			handled := d.renderer.loads.dataError(ctx, "", asHTTPError(err), err)
			body, encodeErr := jsonBytes(errorNodeExtra(handled))
			if encodeErr != nil {
				return encodeErr
			}
			return json.NewEncoder(w).Encode(map[string]any{"type": "error", "kind": "app", "error": &HTTPError{Status: handled.Status, Message: handled.Message}, "body": json.RawMessage(body)})
		}
		data, err := encodeRemoteResult(rs.cfg.Transport, map[string]any{"_": value})
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(map[string]any{"type": "result", "data": data})
	}
	switch fn.kind {
	case KindQuery, KindPrerender:
		result := rs.invokePayload(ctx, fn, input.Payload)
		return answer(result.value, result.err)
	case KindBatch:
		calls, err := rs.parsePayloads(input.Payloads)
		if err != nil {
			return err
		}
		values, err := rs.callBatch(ctx, fn, calls)
		if err == nil && len(values) != len(calls) {
			err = fmt.Errorf("skgo: batch result count differs from arguments")
		}
		return answer(values, err)
	case KindLive:
		arg, present, err := remotearg.ParsePayloadWith(input.Payload, rs.codecs())
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = http.NewResponseController(w).Flush()
		err = rs.callLive(ctx, fn, rs.newCall(arg, present), func(value any) error {
			if err := answer(value, nil); err != nil {
				return err
			}
			return http.NewResponseController(w).Flush()
		})
		if err != nil && ctx.Err() == nil {
			return answer(nil, err)
		}
		return nil
	case KindForm:
		id := actionID(request.URL)
		base, _, _ := splitActionID(id)
		if base != fn.id {
			return fmt.Errorf("skgo: native form callback does not match page submission")
		}
		// Browser remote HTTP bypasses Kit. This callback is the unenhanced page
		// POST; reuse its Go execution with files, keys and empty requested sets.
		caller, err := rs.matchCaller(request)
		if err != nil {
			return answer(nil, err)
		}
		clone := request.Clone(callerContext(ctx, caller))
		if request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return err
			}
			clone.Body = body
			defer body.Close()
		}
		action, redirect, appError := d.renderer.runFormAction(clone, id)
		if redirect != nil {
			return answer(nil, redirect)
		}
		if appError != nil {
			return answer(nil, appError)
		}
		value, _ := action.output.Get("result")
		issues, invalid := action.output.Get("issues")
		data, err := encodeRemoteResult(rs.cfg.Transport, map[string]any{"_": value, "issues": issues})
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(map[string]any{"type": "result", "data": data, "invalid": invalid, "cookies": prerenderCookies(action.jar)})
	default:
		return fmt.Errorf("skgo: native command callback is unavailable outside Kit mutation context")
	}
}
