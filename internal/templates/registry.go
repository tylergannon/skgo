package templates

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/templateapi"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var optionName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func validate(d templateapi.Descriptor) error {
	if !identifier.MatchString(d.ID) || d.Version == "" || d.SkgoVersion == "" {
		return fmt.Errorf("descriptor requires an identifier, version and skgo version")
	}
	if len(d.Addons)+len(d.Templates) == 0 {
		return fmt.Errorf("descriptor exports no add-ons or templates")
	}
	check := func(name, summary string, opts []templateapi.Option, seen map[string]bool) error {
		if !identifier.MatchString(name) || summary == "" || seen[name] {
			return fmt.Errorf("invalid or duplicate export %q", name)
		}
		seen[name] = true
		keys := map[string]bool{}
		for _, o := range opts {
			if !optionName.MatchString(o.Name) || o.Help == "" || keys[o.Name] {
				return fmt.Errorf("invalid or duplicate option %q for %s", o.Name, name)
			}
			keys[o.Name] = true
		}
		return nil
	}
	seen := map[string]bool{}
	for _, a := range d.Addons {
		if err := check(a.Name, a.Summary, a.Options, seen); err != nil {
			return err
		}
	}
	seen = map[string]bool{}
	for _, t := range d.Templates {
		if err := check(t.Name, t.Summary, t.Options, seen); err != nil {
			return err
		}
		if len(t.Steps) == 0 {
			return fmt.Errorf("template %s has no steps", t.Name)
		}
		options := map[string]bool{}
		for _, o := range t.Options {
			options[o.Name] = true
		}
		for _, s := range t.Steps {
			if !identifier.MatchString(s.Addon) {
				return fmt.Errorf("template %s has invalid add-on %q", t.Name, s.Addon)
			}
			for k, v := range s.Bindings {
				if !options[v] {
					return fmt.Errorf("template %s binds undeclared option %q", t.Name, v)
				}
				if _, ok := s.Options[k]; ok {
					return fmt.Errorf("template %s has conflicting binding/literal for %s", t.Name, k)
				}
			}
		}
	}
	return nil
}

func resolve(opts []templateapi.Option, values map[string]string) (map[string]string, error) {
	result := map[string]string{}
	for _, o := range opts {
		result[o.Name] = o.Default
	}
	for k, v := range values {
		if _, ok := result[k]; !ok {
			return nil, fmt.Errorf("unknown option %q", k)
		}
		result[k] = v
	}
	for _, o := range opts {
		if o.Required && result[o.Name] == "" {
			return nil, fmt.Errorf("required option %q is missing", o.Name)
		}
	}
	return result, nil
}

func (r *Registry) addon(name string) (Entry, templateapi.Addon, error) {
	var found []Entry
	var addon templateapi.Addon
	for _, e := range r.Entries {
		for _, a := range e.Descriptor.Addons {
			if a.Name == name {
				found = append(found, e)
				addon = a
			}
		}
	}
	if err := selection("add-on", name, found); err != nil {
		return Entry{}, addon, err
	}
	return found[0], addon, nil
}
func (r *Registry) template(name string) (templateapi.Template, error) {
	var found []Entry
	var tmpl templateapi.Template
	for _, e := range r.Entries {
		for _, t := range e.Descriptor.Templates {
			if t.Name == name {
				found = append(found, e)
				tmpl = t
			}
		}
	}
	return tmpl, selection("template", name, found)
}
func selection(kind, name string, found []Entry) error {
	if len(found) == 0 {
		return fmt.Errorf("%s %q is unavailable; see help and plugin diagnostics", kind, name)
	}
	if len(found) > 1 {
		var paths []string
		for _, e := range found {
			paths = append(paths, e.Path)
		}
		return fmt.Errorf("%s %q is ambiguous: %s; remove the manual provider or use skgo plugin remove MODULE for its managed selection", kind, name, strings.Join(paths, ", "))
	}
	return nil
}

type Operation struct {
	entry   Entry
	Name    string
	Options map[string]string
}

func (r *Registry) Add(name string, values map[string]string) ([]Operation, error) {
	e, a, err := r.addon(name)
	if err != nil {
		return nil, err
	}
	v, err := resolve(a.Options, values)
	if err != nil {
		return nil, fmt.Errorf("add-on %s: %w", name, err)
	}
	return []Operation{{entry: e, Name: name, Options: v}}, nil
}
func (r *Registry) Template(name string, values map[string]string) ([]Operation, error) {
	t, err := r.template(name)
	if err != nil {
		return nil, err
	}
	v, err := resolve(t.Options, values)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	var operations []Operation
	for _, s := range t.Steps {
		args := map[string]string{}
		for k, value := range s.Options {
			args[k] = value
		}
		for k, key := range s.Bindings {
			args[k] = v[key]
		}
		op, err := r.Add(s.Addon, args)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		operations = append(operations, op...)
	}
	return operations, nil
}

// Apply stops at the first failure and prints partial results before returning.
func Apply(ctx context.Context, ops []Operation, project templateapi.Project, out io.Writer) error {
	for i, op := range ops {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := apply(ctx, op, project)
		for _, p := range result.Changed {
			if !filepath.IsLocal(p) {
				return fmt.Errorf("add-on %s reported a non-project-relative path %q (application may be partial)", op.Name, p)
			}
			fmt.Fprintf(out, "%s: changed %s\n", op.Name, p)
		}
		for _, next := range result.Next {
			fmt.Fprintf(out, "%s: %s\n", op.Name, next)
		}
		if err != nil {
			return fmt.Errorf("step %d/%d (%s) failed after %d completed steps: %w; project may contain partial changes; repair the reported conflict or restore the prior state before retrying", i+1, len(ops), op.Name, i, err)
		}
		fmt.Fprintf(out, "applied %s\n", op.Name)
	}
	return nil
}
func apply(ctx context.Context, op Operation, project templateapi.Project) (result templateapi.Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("plugin panicked: %v; changed paths may be incomplete", p)
		}
	}()
	return op.entry.Plugin.Apply(ctx, templateapi.Request{Addon: op.Name, Project: project, Options: op.Options})
}

func (r *Registry) Help(out io.Writer, kind, name string) error {
	fmt.Fprintln(out, "Plugins are trusted native code, loaded eagerly in sorted file order. They must match this host toolchain and shared dependencies; dependencies shared only between plugins must also match. Incompatible plugins cannot coexist: remove conflicting manual files or use skgo plugin remove MODULE for current-host managed installations. SKGO_PLUGIN_DIRS only changes manual discovery.")
	return r.help(out, kind, name)
}

func (r *Registry) help(out io.Writer, kind, name string) error {
	printOptions := func(options []templateapi.Option) {
		for _, o := range options {
			fmt.Fprintf(out, "    --set %s=VALUE: %s (default %q, required %t)\n", o.Name, o.Help, o.Default, o.Required)
		}
	}
	if name != "" {
		if kind == "template" {
			t, err := r.template(name)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s: %s\n", t.Name, t.Summary)
			printOptions(t.Options)
		} else {
			_, a, err := r.addon(name)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s: %s\n", a.Name, a.Summary)
			printOptions(a.Options)
		}
		return nil
	}
	names := map[string]bool{}
	for _, e := range r.Entries {
		if kind == "template" {
			for _, t := range e.Descriptor.Templates {
				names[t.Name] = true
			}
		} else {
			for _, a := range e.Descriptor.Addons {
				names[a.Name] = true
			}
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	fmt.Fprintf(out, "Available %ss:\n", kind)
	for _, name := range ordered {
		if err := r.help(out, kind, name); err != nil {
			fmt.Fprintln(out, err)
		}
	}
	return nil
}
