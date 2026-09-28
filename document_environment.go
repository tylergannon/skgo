package skgo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
)

// environmentJSON is copied into each renderer before its first app module.
func environmentJSON(snapshot *EnvironmentSnapshot) []byte {
	values := map[string]any{"public": map[string]any{}, "private": map[string]any{}, "dynamicPublic": map[string]any{}}
	if snapshot != nil {
		values["public"] = snapshot.Public()
		values["private"] = snapshot.Private()
		values["dynamicPublic"] = snapshot.DynamicPublic()
	}
	// EnvironmentSnapshot contains only the validated JSON primitive types.
	encoded, _ := json.Marshal(values)
	return encoded
}

// serveEnvironment mirrors Kit's env_module.js. Only dynamic public values
// belong here: static exports are already literals in the client bundle.
func (s *SSR) serveEnvironment(w http.ResponseWriter, r *http.Request) {
	if rejectReservedQuery(w, r, false, false) {
		return
	}
	values := map[string]any{}
	if s.environment != nil {
		values = s.environment.DynamicPublic()
	}
	source, err := unevalJSON(values)
	if err != nil {
		http.Error(w, "Cannot serialize public environment", http.StatusInternalServerError)
		return
	}
	body := []byte("export const env=" + source)
	etag := "W/" + etagOf(body)
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func requireEnvironment(build fs.FS, snapshot *EnvironmentSnapshot) error {
	data, err := fs.ReadFile(build, "env.json")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var schema struct {
		Fields []json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("skgo: invalid environment build: %w", err)
	}
	if len(schema.Fields) > 0 && snapshot == nil {
		return fmt.Errorf("skgo: this build declares environment variables; pass LoadEnvironment's snapshot in SSROptions.Environment")
	}
	return nil
}

// Kit interpolates numbers with JavaScript String(number). JSON's numeric
// formatting uses the same decimal/exponent thresholds; normalize negative zero.
func environmentText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if number, ok := value.(float64); ok && number == 0 {
		return "0"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
