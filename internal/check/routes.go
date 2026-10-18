package check

import (
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

// packages is ./... plus every route link package, which ./... skips.
func (v routeInventory) packages() []string {
	out := []string{"./..."}
	for _, link := range v.Links {
		out = append(out, "./"+filepath.ToSlash(link.Link))
	}
	return out
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
