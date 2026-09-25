package skgo

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/tylergannon/polytype/devalue"
)

// classicResult is the same action data delivered in Kit's three channels:
// the rendered form prop, the enhanced ActionResult, and the boot form value.
type classicResult struct {
	tree      any
	wire      string
	status    int
	failure   bool
	jar       *cookieJar
	headers   http.Header
	location  string
	redirect  *Redirect
	actionErr error
}

func classicFormWire(result *classicResult) any {
	if result == nil || result.tree == nil {
		return nil
	}
	return result.wire
}

func (s *SSR) runClassicAction(r *http.Request, route *dataRoute, params map[string]string) (result *classicResult, problem *HTTPError) {
	location := actionLocation(r.URL)
	actionError := func(err error) (*classicResult, *HTTPError) {
		return &classicResult{location: location, jar: newCookieJar(r, secureCookieDefault(s.loads.cfg.Origin, s.loads.cfg.Dev)), headers: http.Header{}, actionErr: err}, nil
	}
	if len(route.nodes) == 0 {
		return actionError(&HTTPError{Status: 405, Message: "POST method not allowed. No form actions exist for this page"})
	}
	leaf := route.nodes[len(route.nodes)-1]
	s.loads.mu.RLock()
	module := ""
	if leaf >= 0 && leaf < len(s.loads.cfg.Nodes) {
		module = s.loads.cfg.Nodes[leaf]
	}
	s.loads.mu.RUnlock()
	if s.actions == nil || len(s.actions.byModule[module]) == 0 {
		return actionError(&HTTPError{Status: 405, Message: "POST method not allowed. No form actions exist for this page"})
	}
	name := "default"
	for _, entry := range strings.Split(r.URL.RawQuery, "&") {
		key, _, _ := strings.Cut(entry, "=")
		key, _ = url.QueryUnescape(key)
		if strings.HasPrefix(key, "/") {
			name = strings.TrimPrefix(key, "/")
			if name == "default" {
				return actionError(errors.New("Cannot use reserved action name \"default\""))
			}
			break
		}
	}
	action := s.actions.lookup(module, name)
	if action == nil {
		return actionError(&HTTPError{Status: 404, Message: "No action with name '" + name + "' found"})
	}
	if !isFormContentType(mediaType(r.Header.Get("Content-Type"))) {
		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "null"
		}
		return actionError(&HTTPError{Status: 415, Message: "Form actions expect form-encoded data — received " + contentType})
	}
	jar := newCookieJar(r, secureCookieDefault(s.loads.cfg.Origin, s.loads.cfg.Dev))
	response := &loadRequest{headers: http.Header{}}
	ctx := withEvent(r.Context(), &Event{req: r, jar: jar, mutable: true, params: params, actionResponse: response})
	defer func() {
		if recovered := recover(); recovered != nil {
			result = &classicResult{jar: jar, headers: response.headers, location: location, actionErr: errors.New("action panicked")}
			problem = nil
		}
	}()
	value, err := action.run(ctx)
	var failure *actionFailure
	if err != nil {
		if !errors.As(err, &failure) {
			result = &classicResult{jar: jar, headers: response.headers, location: location}
			if redirect := asRedirect(err); redirect != nil {
				result.redirect = redirect
			} else {
				result.actionErr = err
			}
			return result, nil
		}
		value = failure.data
	}
	var tree any
	var wire string
	if value != nil {
		var encodeErr error
		tree, encodeErr = s.loads.cfg.Transport.encodeLoadValue(value)
		if encodeErr != nil {
			return nil, &HTTPError{Status: 500, Message: "Internal Error"}
		}
		wire, encodeErr = devalue.StringifyWith(tree, s.loads.cfg.Transport.reducers())
		if encodeErr != nil {
			return nil, &HTTPError{Status: 500, Message: "Internal Error"}
		}
	}
	result = &classicResult{tree: tree, wire: wire, jar: jar, headers: response.headers, location: location, status: 200}
	if failure != nil {
		result.status = failure.status
		result.failure = true
	}
	return result, nil
}

func actionLocation(u *url.URL) string {
	location := u.Path
	removed := false
	for _, entry := range strings.Split(u.RawQuery, "&") {
		if entry == "" {
			continue
		}
		key, _, _ := strings.Cut(entry, "=")
		key, _ = url.QueryUnescape(key)
		if strings.HasPrefix(key, "/") && !removed {
			removed = true
			continue
		}
		if strings.Contains(location, "?") {
			location += "&"
		} else {
			location += "?"
		}
		location += entry
	}
	return location
}

func (s *SSR) writeActionJSON(w http.ResponseWriter, r *http.Request, req dataRequest, route *dataRoute, params map[string]string, result *classicResult) {
	result.jar.writeTo(w.Header())
	for name, values := range result.headers {
		w.Header()[name] = append([]string(nil), values...)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	if s.version != "" {
		w.Header().Set("X-Sveltekit-Version", s.version)
	}
	type actionJSON struct {
		Type     string `json:"type"`
		Status   int    `json:"status,omitempty"`
		Location string `json:"location"`
		Data     string `json:"data,omitempty"`
		Error    any    `json:"error,omitempty"`
	}
	response := actionJSON{Type: "success", Status: 200, Location: result.location, Data: result.wire}
	if result.redirect != nil {
		response = actionJSON{Type: "redirect", Status: result.redirect.status(), Location: result.redirect.Location}
	} else if result.actionErr != nil {
		fallback := asHTTPError(result.actionErr)
		shared := &loadRequest{req: r, jar: result.jar, url: req.url, routeID: route.id, params: params}
		public := s.documentError(hookContext(r, shared), route.id, fallback, result.actionErr)
		response = actionJSON{Type: "error", Location: result.location, Error: public}
		w.WriteHeader(public.Status)
	} else if result.failure {
		response.Type = "failure"
		response.Status = result.status
		w.WriteHeader(result.status)
	} else if result.tree == nil {
		response.Status = 204
	}
	_ = json.NewEncoder(w).Encode(response)
}
