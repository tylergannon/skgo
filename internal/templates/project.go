package templates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tylergannon/skgo/templateapi"
	"golang.org/x/mod/modfile"
)

type Identity struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

// Project resolves identity without writes. The bool says an older project needs
// its explicit name persisted after command/recipe validation.
func Project(root, name string) (templateapi.Project, bool, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return templateapi.Project{}, false, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return templateapi.Project{}, false, err
	}
	p := templateapi.Project{Root: root, Web: filepath.Join(root, "web")}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return p, false, err
	}
	mod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return p, false, err
	}
	if mod.Module == nil {
		return p, false, fmt.Errorf("go.mod has no module identity")
	}
	p.Module = mod.Module.Mod.Path
	if st, err := os.Stat(p.Web); err != nil || !st.IsDir() {
		return p, false, fmt.Errorf("%s is not a skgo web directory", p.Web)
	}
	data, err = os.ReadFile(filepath.Join(root, "skgo.json"))
	missing := os.IsNotExist(err)
	if err != nil && !missing {
		return p, false, err
	}
	if !missing {
		var id Identity
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&id); err != nil {
			return p, false, fmt.Errorf("skgo.json: %w", err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return p, false, fmt.Errorf("skgo.json must contain one identity")
		}
		if id.Version != 1 || !identifier.MatchString(id.Name) {
			return p, false, fmt.Errorf("skgo.json has invalid version or name")
		}
		if name != "" && name != id.Name {
			return p, false, fmt.Errorf("--name %q disagrees with existing name %q", name, id.Name)
		}
		name = id.Name
	} else {
		if name == "" {
			return p, false, fmt.Errorf("older project has no skgo.json: pass --name explicitly")
		}
		if _, err := os.Stat(filepath.Join(root, "native")); err == nil {
			return p, false, fmt.Errorf("older project has native identity: verify its display name and bundle configuration, then create skgo.json with version 1 and that name before adding capabilities")
		}
	}
	if !identifier.MatchString(name) {
		return p, false, fmt.Errorf("invalid application name %q", name)
	}
	p.Name = name
	return p, missing, nil
}

func SaveIdentity(p templateapi.Project) error {
	data, err := json.MarshalIndent(Identity{Version: 1, Name: p.Name}, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(p.Root, "skgo.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
