package check

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type routeLink struct {
	Link   string `json:"link"`
	Target string `json:"target"`
}
type routeInventory struct {
	Links []routeLink `json:"links"`
}

func loadRoutes(out string) (routeInventory, error) {
	b, err := os.ReadFile(filepath.Join(out, "links.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return routeInventory{}, nil
	}
	if err != nil {
		return routeInventory{}, err
	}
	var v routeInventory
	if err := json.Unmarshal(b, &v); err != nil {
		return v, err
	}
	return v, nil
}

// makeOverlay gives the Go toolchain the current authored route bytes while
// leaving committed generated link copies untouched. The temporary JSON file
// is removed by the caller before check returns.
func makeOverlay(root string, v routeInventory) (string, func(), error) {
	replace := map[string]string{}
	for _, link := range v.Links {
		dir := filepath.Join(root, filepath.FromSlash(link.Target))
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			source := filepath.Join(dir, entry.Name())
			copy := filepath.Join(root, filepath.FromSlash(link.Link), entry.Name())
			a, err := os.ReadFile(source)
			if err != nil {
				return "", nil, err
			}
			b, _ := os.ReadFile(copy)
			if !bytes.Equal(a, b) {
				replace[copy] = source
			}
		}
	}
	if len(replace) == 0 {
		return "", func() {}, nil
	}
	f, err := os.CreateTemp(root, ".skgo-check-overlay-*.json")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	if err := json.NewEncoder(f).Encode(struct {
		Replace map[string]string `json:"Replace"`
	}{replace}); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return f.Name(), cleanup, nil
}

func authoredPath(root string, v routeInventory, path string) string {
	clean := strings.TrimPrefix(relative(root, path), "./")
	for _, link := range v.Links {
		prefix := filepath.ToSlash(link.Link) + "/"
		if strings.HasPrefix(clean, prefix) {
			return filepath.ToSlash(link.Target) + "/" + strings.TrimPrefix(clean, prefix)
		}
	}
	return clean
}

func authoredMessage(root string, v routeInventory, message string) string {
	for _, link := range v.Links {
		generated := filepath.ToSlash(filepath.Join(root, filepath.FromSlash(link.Link)))
		authored := filepath.ToSlash(filepath.Join(root, filepath.FromSlash(link.Target)))
		message = strings.ReplaceAll(message, generated+"/", authored+"/")
		message = strings.ReplaceAll(message, filepath.ToSlash(link.Link)+"/", filepath.ToSlash(link.Target)+"/")
	}
	return message
}
