package skgo

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"github.com/tylergannon/skgo/internal/envspec"
)

// Environment declares the application's environment in src/env.go. T must be a
// struct with exported string, bool, int or float64 fields tagged
// `env:"NAME[,public][,static]"`. Pointer fields are optional. Strings retain
// their exact contents; booleans accept true/false and yes/no, case-insensitively.
// Integers use base ten and must fit both Go int and JavaScript's safe integer
// range. float64 values must be finite decimal numbers. Whitespace is not
// trimmed. Unset required fields fail at build and startup; an empty string is
// valid only for string fields. Static fields bind at build, dynamic fields at
// startup. This marker does not read values. Run skgo generate after editing it.
func Environment[T any]() struct{} { return struct{}{} }

// EnvironmentSnapshot contains the values validated once for this application.
// Accessors return copies so callers cannot change the renderer's environment.
type EnvironmentSnapshot struct {
	devToken string
	schema   envspec.Schema
	values   map[string]any
}

func (s *EnvironmentSnapshot) project(public, dynamic bool) map[string]any {
	result := map[string]any{}
	if s == nil {
		return result
	}
	for _, f := range s.schema.Fields {
		if f.Public == public && (!dynamic || !f.Static) {
			if v, ok := s.values[f.Name]; ok {
				result[f.Name] = v
			}
		}
	}
	return result
}

// Public returns every public value, including static values.
func (s *EnvironmentSnapshot) Public() map[string]any { return s.project(true, false) }

// Private returns every private value.
func (s *EnvironmentSnapshot) Private() map[string]any { return s.project(false, false) }

// DynamicPublic returns public values whose binding time is startup.
func (s *EnvironmentSnapshot) DynamicPublic() map[string]any { return s.project(true, true) }

// LoadEnvironment validates startup values using the same parser used at build
// time. Static values come exclusively from env.json in the embedded build.
// lookup is normally os.LookupEnv. Errors name variables but never print values.
func LoadEnvironment[T any](build fs.FS, lookup func(string) (string, bool)) (T, *EnvironmentSnapshot, error) {
	var result T
	typ := reflect.TypeFor[T]()
	if typ.Kind() != reflect.Struct {
		return result, nil, fmt.Errorf("skgo: environment type must be a struct")
	}
	data, err := fs.ReadFile(build, "env.json")
	if err != nil {
		return result, nil, fmt.Errorf("skgo: read environment build: %w", err)
	}
	var schema envspec.Schema
	if err = json.Unmarshal(data, &schema); err != nil {
		return result, nil, fmt.Errorf("skgo: invalid environment build")
	}

	return loadEnvironmentSchema[T](schema, lookup)
}

func loadEnvironmentSchema[T any](schema envspec.Schema, lookup func(string) (string, bool)) (T, *EnvironmentSnapshot, error) {
	var result T
	typ := reflect.TypeFor[T]()
	if typ.Kind() != reflect.Struct {
		return result, nil, fmt.Errorf("skgo: environment type must be a struct")
	}
	if len(schema.Fields) != typ.NumField() {
		return result, nil, fmt.Errorf("skgo: environment declaration differs from build; regenerate and rebuild")
	}
	for i, f := range schema.Fields {
		sf := typ.Field(i)
		ft := sf.Type
		optional := ft.Kind() == reflect.Pointer
		if optional {
			ft = ft.Elem()
		}
		decl, err := envspec.ParseTag(sf.Name, ft.String(), sf.Tag.Get("env"), optional)
		if err != nil {
			return result, nil, err
		}
		if !sf.IsExported() || decl != f {
			return result, nil, fmt.Errorf("skgo: environment declaration differs from build at %s; regenerate and rebuild", sf.Name)
		}
	}
	values, err := schema.Resolve(lookup, true)
	if err != nil {
		return result, nil, err
	}
	rv := reflect.ValueOf(&result).Elem()
	for i, f := range schema.Fields {
		v, ok := values[f.Name]
		if !ok {
			continue
		}
		dst := rv.Field(i)
		if f.Optional {
			dst.Set(reflect.New(dst.Type().Elem()))
			dst = dst.Elem()
		}
		encoded, err := json.Marshal(v)
		if err != nil {
			return result, nil, fmt.Errorf("skgo: invalid build value for %s", f.Name)
		}
		if err = json.Unmarshal(encoded, dst.Addr().Interface()); err != nil {
			return result, nil, fmt.Errorf("skgo: invalid build value for %s", f.Name)
		}
	}
	return result, &EnvironmentSnapshot{schema: schema, values: values}, nil
}

// LoadDevEnvironment reads Vite's resolved development environment. Start the
// Vite development server first; restart both servers after changing .env files.
// The adapter keeps this private snapshot outside browser-served files.
func LoadDevEnvironment[T any](web string) (T, *EnvironmentSnapshot, error) {
	var zero T
	data, err := os.ReadFile(filepath.Join(web, ".svelte-kit", "skgo-env-runtime.json"))
	if err != nil {
		return zero, nil, fmt.Errorf("skgo: read development environment: %w", err)
	}
	var payload struct {
		envspec.Schema
		Values map[string]string `json:"values"`
		Token  string            `json:"token"`
	}
	if err = json.Unmarshal(data, &payload); err != nil {
		return zero, nil, fmt.Errorf("skgo: invalid development environment")
	}
	result, snapshot, err := loadEnvironmentSchema[T](payload.Schema, func(name string) (string, bool) { v, ok := payload.Values[name]; return v, ok })
	if snapshot != nil {
		snapshot.devToken = payload.Token
	}
	return result, snapshot, err
}
