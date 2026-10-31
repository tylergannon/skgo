// Package windowsapp creates an application-owned WinUI shell. The Go plugin
// runs on the developer's machine; generated applications never load it.
package windowsapp

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/templateapi"
)

//go:embed files/windows
var files embed.FS

var identity = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9.-]{2,49}$`)

func Apply(ctx context.Context, r templateapi.Request) (result templateapi.Result, err error) {
	if !filepath.IsAbs(r.Project.Root) {
		return result, fmt.Errorf("Windows setup requires an absolute project root")
	}
	r.Project.Root = filepath.Clean(r.Project.Root)
	if r.Addon != "windows-app" {
		return result, fmt.Errorf("unknown Windows add-on %q", r.Addon)
	}
	id := r.Options["identity"]
	if !identity.MatchString(id) {
		return result, fmt.Errorf("identity must be 3–50 letters, digits, dots or hyphens, beginning with a letter")
	}
	planned := map[string][]byte{}
	err = fs.WalkDir(files, "files", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		planned[strings.TrimPrefix(name, "files/")] = bytes.ReplaceAll(b, []byte("SKGo.Application"), []byte(id))
		return nil
	})
	if err != nil {
		return result, err
	}
	planned["windows/.gitignore"] = []byte("SDK/\nnative/\nbin/\nobj/\nbuild/\n*.pfx\n")
	planned["windows/skgo-windows.json"], _ = json.MarshalIndent(struct {
		Version  int    `json:"version"`
		Identity string `json:"identity"`
	}{1, id}, "", "  ")
	for _, size := range []int{44, 150} {
		icon := image.NewNRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				c := color.NRGBA{R: 28, G: 75, B: 120, A: 255}
				if x > size/4 && x < size*3/4 && y > size/4 && y < size*3/4 {
					c = color.NRGBA{R: 245, G: 248, B: 252, A: 255}
				}
				icon.SetNRGBA(x, y, c)
			}
		}
		var b bytes.Buffer
		if err := png.Encode(&b, icon); err != nil {
			return result, err
		}
		planned[fmt.Sprintf("windows/Assets/Logo%d.png", size)] = b.Bytes()
	}
	// Preflight the entire tree. Setup is create-only; once configured, all
	// authored files belong to the app, including its project and scripts.
	names := make([]string, 0, len(planned))
	for n := range planned {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for p := filepath.Join(r.Project.Root, filepath.FromSlash(n)); p != r.Project.Root; p = filepath.Dir(p) {
			st, e := os.Lstat(p)
			if e == nil && st.Mode()&os.ModeSymlink != 0 {
				return result, fmt.Errorf("%s contains a symlink", n)
			}
			if e != nil && !os.IsNotExist(e) {
				return result, e
			}
		}
	}
	configured := false
	config := filepath.Join(r.Project.Root, "windows/skgo-windows.json")
	if b, e := os.ReadFile(config); e == nil {
		var prior struct {
			Version  int    `json:"version"`
			Identity string `json:"identity"`
		}
		if json.Unmarshal(b, &prior) != nil || prior.Version != 1 || prior.Identity != id {
			return result, fmt.Errorf("windows/skgo-windows.json conflicts with requested identity")
		}
		configured = true
	} else if !os.IsNotExist(e) {
		return result, e
	}
	for _, n := range names {
		b, e := os.ReadFile(filepath.Join(r.Project.Root, filepath.FromSlash(n)))
		if e == nil {
			if !configured && !bytes.Equal(b, planned[n]) {
				return result, fmt.Errorf("%s conflicts with existing content", n)
			}
			delete(planned, n)
		} else if !os.IsNotExist(e) {
			return result, e
		}
	}
	for _, n := range names {
		b, ok := planned[n]
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		p := filepath.Join(r.Project.Root, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return result, err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return result, err
		}
		result.Changed = append(result.Changed, n)
		_, err = f.Write(b)
		closeErr := f.Close()
		if err != nil {
			return result, err
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	result.Next = []string{"Run skgo plugin windows-shell prepare, then run windows/build.ps1 on Windows 11 x64."}
	return result, nil
}
