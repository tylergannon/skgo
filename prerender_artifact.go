package skgo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo/internal/ssr"
)

// prerenderArtifactStore is the authority Kit's production prerender wrapper
// uses before it validates an argument or invokes the declaration's body.
// Client files shadow prerendered files, matching adapter-node's middleware
// order; the manifest remains the authority for the prerendered tree.
type prerenderArtifactStore struct {
	build    fs.FS
	manifest Manifest
	client   map[string]assetMeta
	recorded map[string]prerenderArtifact
	decoded  map[string]prerenderArtifact
}

type prerenderArtifact struct {
	path   string
	file   assetMeta
	result any
	err    *ssr.Error
}

type prerenderArtifactEnvelope struct {
	Type  string          `json:"type"`
	Data  string          `json:"data"`
	Error json.RawMessage `json:"error"`
}

func remoteTransport(remotes *Remotes) Transport {
	if remotes == nil {
		return nil
	}
	return remotes.cfg.Transport
}

func newPrerenderArtifactStore(build fs.FS, manifest Manifest, client, prerendered map[string]assetMeta) (*prerenderArtifactStore, error) {
	if manifest.AppDir == "" {
		manifest.AppDir = "_app"
	}
	base := strings.TrimSuffix(manifest.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	prefix := base + "/" + manifest.AppDir + "/remote/"
	store := &prerenderArtifactStore{build: build, manifest: manifest, client: client, recorded: map[string]prerenderArtifact{}, decoded: map[string]prerenderArtifact{}}
	for _, urlPath := range manifest.Prerendered {
		if !strings.HasPrefix(urlPath, prefix) || strings.Contains(urlPath, "?") {
			continue
		}
		rest := strings.TrimPrefix(urlPath, prefix)
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("skgo: malformed prerendered remote path %q", urlPath)
		}
		payload := ""
		if len(parts) == 3 {
			payload = parts[2]
		}
		id := parts[0] + "/" + parts[1]
		meta, _, ok := resolvePrerenderedMeta(prerendered, urlPath, base)
		if !ok || len(meta.variants) == 0 {
			return nil, fmt.Errorf("skgo: recorded prerendered remote %q has no file", urlPath)
		}
		data, err := fs.ReadFile(build, meta.variants[0].name)
		if err != nil {
			return nil, fmt.Errorf("skgo: reading recorded prerendered remote %q: %w", urlPath, err)
		}
		if err := validatePrerenderArtifactSyntax(data, urlPath); err != nil {
			return nil, err
		}
		key := id + "\x00" + payload
		if previous, exists := store.recorded[key]; exists {
			return nil, fmt.Errorf("skgo: duplicate prerendered remote paths %q and %q", previous.path, urlPath)
		}
		store.recorded[key] = prerenderArtifact{path: urlPath, file: meta}
	}
	return store, nil
}

func validatePrerenderArtifactSyntax(data []byte, identifier string) error {
	var envelope prerenderArtifactEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("skgo: invalid prerendered remote artifact %q: %w", identifier, err)
	}
	switch envelope.Type {
	case "result", "redirect":
		var wire any
		if envelope.Data == "" || json.Unmarshal([]byte(envelope.Data), &wire) != nil {
			return fmt.Errorf("skgo: invalid prerendered remote artifact %q: result has invalid data", identifier)
		}
	case "error":
		if err := validatePrerenderErrorBody(envelope.Error); err != nil {
			return fmt.Errorf("skgo: invalid prerendered remote artifact %q: error has invalid body: %w", identifier, err)
		}
	default:
		return fmt.Errorf("skgo: invalid prerendered remote artifact %q: unknown type %q", identifier, envelope.Type)
	}
	return nil
}

// validatePrerenderErrorBody enforces Kit's required App.Error fields without
// imposing an HTTP status range or converting its number. Static serving only
// validates the wire shape; SSR's int-backed Error decoder separately rejects
// a fractional or otherwise unrepresentable status rather than truncating it.
func validatePrerenderErrorBody(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("error body is missing")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		return err
	}
	if body == nil {
		return errors.New("error body is not an object")
	}
	if _, ok := body["status"].(json.Number); !ok {
		return errors.New("error body has no numeric status")
	}
	if _, ok := body["message"].(string); !ok {
		return errors.New("error body has no string message")
	}
	return nil
}

func resolvePrerenderedMeta(files map[string]assetMeta, urlPath, base string) (assetMeta, string, bool) {
	rel := strings.TrimPrefix(strings.TrimPrefix(urlPath, base), "/")
	candidates := []string{rel, rel + ".html", path.Join(rel, "index.html")}
	for _, candidate := range candidates {
		if meta, ok := files[candidate]; ok {
			return meta, candidate, true
		}
	}
	return assetMeta{}, "", false
}

func (s *prerenderArtifactStore) lookup(id, payload string, transport Transport) (prerenderArtifact, bool, error) {
	if s == nil {
		return prerenderArtifact{}, false, nil
	}
	manifest := s.manifest
	appDir := manifest.AppDir
	if appDir == "" {
		appDir = "_app"
	}
	logical := appDir + "/remote/" + id
	if payload != "" {
		logical += "/" + payload
	}
	if meta, ok := s.client[logical]; ok && len(meta.variants) > 0 {
		data, err := fs.ReadFile(s.build, meta.variants[0].name)
		if err != nil {
			return prerenderArtifact{}, true, fmt.Errorf("skgo: reading client remote artifact %q: %w", logical, err)
		}
		artifact, err := decodePrerenderArtifact(data, logical, transport.revivers())
		return artifact, true, err
	}
	key := id + "\x00" + payload
	registered, ok := s.recorded[key]
	if !ok {
		return prerenderArtifact{}, false, nil
	}
	if artifact, exists := s.decoded[key]; exists {
		return artifact, true, nil
	}
	if len(registered.file.variants) == 0 {
		return prerenderArtifact{}, true, fmt.Errorf("skgo: recorded prerendered remote %q has no file", registered.path)
	}
	data, err := fs.ReadFile(s.build, registered.file.variants[0].name)
	if err != nil {
		return prerenderArtifact{}, true, fmt.Errorf("skgo: reading recorded prerendered remote %q: %w", registered.path, err)
	}
	artifact, err := decodePrerenderArtifact(data, registered.path, transport.revivers())
	return artifact, true, err
}

func newSSRPrerenderArtifactStore(build fs.FS, manifest Manifest, remotes *Remotes, transport Transport) (*prerenderArtifactStore, error) {
	client, err := indexTree(build, "client")
	if err != nil {
		return nil, fmt.Errorf("skgo: reading client/ for prerendered remotes: %w", err)
	}
	prerendered := map[string]assetMeta{}
	if _, err := fs.Stat(build, "prerendered"); err == nil {
		prerendered, err = indexTree(build, "prerendered")
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("skgo: reading prerendered/ for remote artifacts: %w", err)
	}
	if len(manifest.PrerenderedFiles) > 0 {
		mapped, err := indexMappedPrerenderedFiles(build, manifest.PrerenderedFiles)
		if err != nil {
			return nil, err
		}
		for logical, meta := range mapped {
			if _, exists := prerendered[logical]; exists {
				return nil, fmt.Errorf("skgo: prerendered file mapping conflicts with prerendered/%s", logical)
			}
			prerendered[logical] = meta
		}
	}
	store, err := newPrerenderArtifactStore(build, manifest, client, prerendered)
	if err != nil {
		return nil, err
	}
	if remotes == nil {
		return store, nil
	}
	for key, record := range store.recorded {
		id := strings.SplitN(key, "\x00", 2)[0]
		fn, ok := remotes.Lookup(id)
		if !ok || fn.kind != KindPrerender {
			continue
		}
		data, err := fs.ReadFile(build, record.file.variants[0].name)
		if err != nil {
			return nil, fmt.Errorf("skgo: reading recorded prerendered remote %q: %w", record.path, err)
		}
		artifact, err := decodePrerenderArtifact(data, record.path, transport.revivers())
		if err != nil {
			return nil, err
		}
		store.decoded[key] = artifact
	}
	return store, nil
}

func decodePrerenderArtifact(data []byte, identifier string, revivers map[string]func(any) (any, error)) (prerenderArtifact, error) {
	var envelope prerenderArtifactEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: %w", identifier, err)
	}
	switch envelope.Type {
	case "result", "redirect":
		if envelope.Data == "" {
			return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: result has no data", identifier)
		}
		value, err := devalue.Parse(envelope.Data, revivers)
		if err != nil {
			return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: %w", identifier, err)
		}
		object, ok := value.(*devalue.Object)
		if !ok {
			return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: result data is not an object", identifier)
		}
		result, ok := object.Get("_")
		if !ok {
			result = devalue.Undefined
		}
		return prerenderArtifact{result: result}, nil
	case "error":
		if len(envelope.Error) == 0 || string(envelope.Error) == "null" {
			return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: error has no body", identifier)
		}
		if err := validatePrerenderErrorBody(envelope.Error); err != nil {
			return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: error has invalid body: %w", identifier, err)
		}
		var body ssr.Error
		if err := json.Unmarshal(envelope.Error, &body); err != nil {
			return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: %w", identifier, err)
		}
		return prerenderArtifact{err: &body}, nil
	default:
		return prerenderArtifact{}, fmt.Errorf("skgo: invalid prerendered remote artifact %q: unknown type %q", identifier, envelope.Type)
	}
}
