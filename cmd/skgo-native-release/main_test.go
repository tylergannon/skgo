package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (candidate, map[string][]byte, []byte, string) {
	t.Helper()
	m := candidate{Run: 42, Attempt: 1, Source: strings.Repeat("a", 40), CoreSource: strings.Repeat("b", 40), Core: "v0.46.0", Plugin: "v0.1.1"}
	m.Artifact = filename(m.Plugin, m.Core)
	plugin := []byte("immutable plugin fixture, not a recompiled candidate\n")
	sum := sha256.Sum256(plugin)
	m.SHA = hex.EncodeToString(sum[:])
	manifest, _ := json.Marshal(m)
	files := map[string][]byte{m.Artifact: plugin, "checksums.txt": []byte(m.SHA + "  " + m.Artifact + "\n"), "candidate.json": manifest, "notes.md": []byte("Frozen consumer instructions"), "toolchain.json": []byte(`{"go":"1.27.3","node":"24.22.0","vite_plus":"1.3.0","pnpm":"12.14.0","zig":"0.17.0","xcodegen":"2.46.0"}`)}
	archive, digest := zipFiles(t, files)
	toolSum := sha256.Sum256(files["toolchain.json"])
	proof, _ := json.MarshalIndent(receipt{request{m, 71, digest}, hex.EncodeToString(toolSum[:])}, "", "  ")
	files["release.json"] = append(proof, '\n')
	return m, files, archive, digest
}
func zipFiles(t *testing.T, files map[string][]byte) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b.Bytes())
	return b.Bytes(), "sha256:" + hex.EncodeToString(sum[:])
}
func workspace(t *testing.T) string { t.Helper(); dir := t.TempDir(); t.Chdir(dir); return dir }
func serve(t *testing.T, handler func(http.ResponseWriter, *http.Request)) client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(s.Close)
	return client{s.URL, "owner/product", "public-token", s.Client()}
}
func send(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func candidateAPI(t *testing.T, m candidate, archive []byte, digest string, extra func(http.ResponseWriter, *http.Request) bool) client {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		if extra != nil && extra(w, r) {
			return
		}
		switch r.URL.Path {
		case "/repos/owner/product/actions/runs/42/attempts/1":
			send(w, run{ID: 42, Attempt: 1, Path: ".github/workflows/native-plugin.yml", Event: "workflow_run", SHA: m.Source, Conclusion: "failure"}) // notification failed, build succeeded
		case "/repos/owner/product/actions/runs/42":
			send(w, run{ID: 42, Attempt: 2, Path: ".github/workflows/native-plugin.yml", Event: "workflow_run", SHA: m.Source, Conclusion: "success"})
		case "/repos/owner/product/actions/runs/42/attempts/1/jobs":
			send(w, map[string]any{"jobs": []map[string]string{{"name": "candidate", "conclusion": "success"}, {"name": "notify", "conclusion": "failure"}}})
		case "/repos/owner/product/actions/runs/42/artifacts":
			send(w, map[string]any{"artifacts": []artifact{{ID: 71, Name: "native-shell-candidate-1", Digest: digest}}})
		case "/repos/owner/product/actions/artifacts/71/zip":
			w.Write(archive)
		case "/repos/owner/product/git/ref/tags/plugins/native-shell/v0.1.1":
			send(w, map[string]any{"object": map[string]string{"type": "commit", "sha": m.Source}})
		case "/repos/owner/product/git/ref/tags/v0.46.0":
			send(w, map[string]any{"object": map[string]string{"type": "commit", "sha": m.CoreSource}})
		case "/repos/owner/product/releases/tags/v0.46.0":
			send(w, release{ID: 80, Tag: m.Core})
		default:
			t.Errorf("unexpected API %s %s", r.Method, r.URL.String())
			http.Error(w, "unexpected", 500)
		}
	})
}
func TestNextVersionIndependentNumericPatch(t *testing.T) {
	for _, tt := range []struct {
		tags []string
		want string
	}{{nil, "v0.1.0"}, {[]string{"v99.0.0", "plugins/native-shell/v0.1.9", "plugins/native-shell/v0.1.10", "plugins/native-shell/v0.2.2-rc.1"}, "v0.1.11"}, {[]string{"plugins/native-shell/v2.4.99", "plugins/native-shell/v3.0.0"}, "v3.0.1"}} {
		got, err := nextVersion(tt.tags)
		if err != nil || got != tt.want {
			t.Fatalf("nextVersion(%v)=%q,%v want %q", tt.tags, got, err, tt.want)
		}
	}
}
func TestReservationNeverOverwritesExistingTag(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "42")
	called := false
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			send(w, map[string]any{"object": map[string]string{"type": "commit", "sha": strings.Repeat("a", 40)}})
			return
		}
		if r.URL.Path == "/repos/owner/product/git/tags" {
			send(w, map[string]string{"sha": strings.Repeat("c", 40)})
			return
		}
		if r.Method != "POST" || r.URL.Path != "/repos/owner/product/git/refs" {
			t.Fatal(r.Method, r.URL)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["ref"] != "refs/tags/plugins/native-shell/v0.1.1" {
			t.Fatal(body)
		}
		called = true
		http.Error(w, "reference already exists", 422)
	})
	if _, err := c.reserve("v0.1.1", strings.Repeat("a", 40)); err == nil || !called {
		t.Fatal("existing version was accepted")
	}
}
func TestAuthorizeOriginalSuccessfulBuildAfterNotificationRerun(t *testing.T) {
	workspace(t)
	m, _, archive, digest := fixture(t)
	c := candidateAPI(t, m, archive, digest, nil)
	if err := c.authorize(request{m, 71, digest}, "candidate"); err != nil {
		t.Fatal(err)
	}
	if err := verifyFiles("candidate", m); err != nil {
		t.Fatal(err)
	}
}
func TestPromotionRejectsChangedManifestFields(t *testing.T) {
	m, _, archive, digest := fixture(t)
	for _, field := range []string{"plugin", "core", "source", "core-source", "sha", "artifact", "attempt", "artifact-id", "artifact-digest"} {
		t.Run(field, func(t *testing.T) {
			workspace(t)
			r := request{m, 71, digest}
			switch field {
			case "plugin":
				r.Plugin = "v0.1.2"
				r.Artifact = filename(r.Plugin, r.Core)
			case "core":
				r.Core = "v0.45.0"
				r.Artifact = filename(r.Plugin, r.Core)
			case "source":
				r.Source = strings.Repeat("c", 40)
			case "core-source":
				r.CoreSource = strings.Repeat("c", 40)
			case "sha":
				r.SHA = strings.Repeat("0", 64)
			case "artifact":
				r.Artifact = "other.so"
			case "attempt":
				r.Attempt = 0
			case "artifact-id":
				r.ArtifactID = 99
			case "artifact-digest":
				r.ArtifactDigest = "sha256:" + strings.Repeat("0", 64)
			}
			c := candidateAPI(t, m, archive, digest, nil)
			if err := c.authorize(r, "candidate"); err == nil {
				t.Fatal("altered promotion accepted")
			}
		})
	}
}
func TestCandidateRejectsCorruptAndUnsafeArchives(t *testing.T) {
	m, files, archive, digest := fixture(t)
	for _, kind := range []string{"archive-digest", "plugin-digest", "checksum-file", "path-traversal"} {
		t.Run(kind, func(t *testing.T) {
			workspace(t)
			copyFiles := map[string][]byte{}
			for k, v := range files {
				copyFiles[k] = v
			}
			a, d := archive, digest
			switch kind {
			case "archive-digest":
				a = append(a, 'x')
			case "plugin-digest":
				copyFiles[m.Artifact] = []byte("changed")
				a, d = zipFiles(t, copyFiles)
			case "checksum-file":
				copyFiles["checksums.txt"] = []byte("anything\n")
				a, d = zipFiles(t, copyFiles)
			case "path-traversal":
				copyFiles["../outside"] = []byte("escape")
				a, d = zipFiles(t, copyFiles)
			}
			c := candidateAPI(t, m, a, d, nil)
			if _, err := c.findCandidate(42, "candidate"); err == nil {
				t.Fatal("invalid archive accepted")
			}
		})
	}
}
func TestConfiguredNotificationHasCompleteIdentity(t *testing.T) {
	workspace(t)
	m, _, archive, digest := fixture(t)
	t.Setenv("NATIVE_RELEASE_DISPATCH_REPOSITORY", "service/qualification")
	t.Setenv("NATIVE_RELEASE_DISPATCH_TOKEN", "dispatch-only-token")
	t.Setenv("GITHUB_RUN_ID", "42")
	called := false
	c := candidateAPI(t, m, archive, digest, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/repos/service/qualification/dispatches" {
			return false
		}
		if r.Header.Get("Authorization") != "Bearer dispatch-only-token" {
			t.Fatal("wrong dispatch credential")
		}
		var event struct {
			Type    string                     `json:"event_type"`
			Payload map[string]json.RawMessage `json:"client_payload"`
		}
		json.NewDecoder(r.Body).Decode(&event)
		if event.Type != "native-plugin-candidate" || len(event.Payload) != 10 {
			t.Fatal(event)
		}
		var actual request
		b, _ := json.Marshal(event.Payload)
		json.Unmarshal(b, &actual)
		if actual != (request{m, 71, digest}) {
			t.Fatal(actual)
		}
		called = true
		w.WriteHeader(204)
		return true
	})
	if err := c.notify(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("notification not delivered")
	}
}
func TestMissingDispatchConfigurationFailsVisibly(t *testing.T) {
	for _, tt := range []struct{ repo, token string }{{"", ""}, {"hardcoded/guess", ""}, {"bad path", "secret"}} {
		t.Setenv("NATIVE_RELEASE_DISPATCH_REPOSITORY", tt.repo)
		t.Setenv("NATIVE_RELEASE_DISPATCH_TOKEN", tt.token)
		if _, _, err := dispatchConfig(); err == nil {
			t.Fatal("unconfigured dispatch accepted")
		}
	}
}
func TestPromotionPublishesOriginalBytesAndRecoversNotification(t *testing.T) {
	workspace(t)
	m, files, archive, digest := fixture(t)
	r := request{m, 71, digest}
	writeJSON("event.json", map[string]any{"client_payload": r})
	t.Setenv("GITHUB_EVENT_PATH", "event.json")
	t.Setenv("NATIVE_RELEASE_DISPATCH_REPOSITORY", "service/qualification")
	t.Setenv("NATIVE_RELEASE_DISPATCH_TOKEN", "dispatch-token")
	published, notifications, creates := false, 0, 0
	rel := release{ID: 91, Tag: "plugins/native-shell/v0.1.1", URL: "https://github.com/owner/product/releases/tag/plugins/native-shell/v0.1.1"}
	c := candidateAPI(t, m, archive, digest, func(w http.ResponseWriter, req *http.Request) bool {
		switch req.URL.Path {
		case "/repos/owner/product/releases/tags/plugins/native-shell/v0.1.1":
			if !published || rel.Draft {
				http.NotFound(w, req)
			} else {
				send(w, rel)
			}
			return true
		case "/repos/owner/product/releases":
			if published {
				send(w, []release{rel})
			} else {
				send(w, []release{})
			}
			return true
		case "/repos/service/qualification/dispatches":
			notifications++
			var body struct {
				Type    string `json:"event_type"`
				Payload struct {
					Candidate request `json:"candidate"`
					ReleaseID int64   `json:"release_id"`
				} `json:"client_payload"`
			}
			json.NewDecoder(req.Body).Decode(&body)
			if body.Type != "native-plugin-published" || body.Payload.Candidate != r || body.Payload.ReleaseID != 91 {
				t.Fatal(body)
			}
			if notifications == 1 {
				http.Error(w, "transient notification error", 503)
			} else {
				w.WriteHeader(204)
			}
			return true
		}
		return false
	})
	originalGH := runGH
	t.Cleanup(func() { runGH = originalGH })
	runGH = func(args ...string) error {
		if len(args) < 2 {
			return fmt.Errorf("missing gh command")
		}
		if args[0] != "release" {
			return fmt.Errorf("unexpected gh %v", args)
		}
		if args[1] == "create" {
			creates++
			if published {
				return fmt.Errorf("attempted overwrite")
			}
			if !strings.Contains(strings.Join(args, " "), "--verify-tag --latest=false --draft") {
				t.Fatal(args)
			}
			for name, data := range files {
				b, err := os.ReadFile(filepath.Join("candidate", name))
				if err != nil || !bytes.Equal(b, data) {
					t.Fatalf("created asset %s differs from original: %v", name, err)
				}
			}
			published = true
			rel.Draft = true
			for _, name := range []string{m.Artifact} {
				var asset struct {
					Name string `json:"name"`
					URL  string `json:"browser_download_url"`
				}
				asset.Name = name
				rel.Assets = append(rel.Assets, asset)
			}
			return fmt.Errorf("transient upload failure after creating partial draft")
		}
		if args[1] == "upload" {
			name := filepath.Base(args[len(args)-1])
			for _, asset := range rel.Assets {
				if asset.Name == name {
					t.Fatal("attempted to overwrite draft asset", name)
				}
			}
			b, err := os.ReadFile(args[len(args)-1])
			if err != nil || !bytes.Equal(b, files[name]) {
				t.Fatal("uploaded bytes differ", err)
			}
			var asset struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}
			asset.Name = name
			rel.Assets = append(rel.Assets, asset)
			return nil
		}
		if args[1] == "edit" {
			if !rel.Draft {
				t.Fatal("attempted to edit published release")
			}
			rel.Draft = false
			return nil
		}
		if args[1] == "download" {
			name, dir := "", ""
			for i := 2; i < len(args)-1; i++ {
				if args[i] == "--pattern" {
					name = args[i+1]
				}
				if args[i] == "--dir" {
					dir = args[i+1]
				}
			}
			return os.WriteFile(filepath.Join(dir, name), files[name], 0644)
		}
		return fmt.Errorf("unexpected gh %v", args)
	}
	if err := c.promote(); err == nil || !strings.Contains(err.Error(), "partial draft") {
		t.Fatalf("partial draft upload must fail visibly: %v", err)
	}
	if creates != 1 || notifications != 0 {
		t.Fatal("partial draft was published or notified")
	}
	if err := c.promote(); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("first notification should fail visibly: %v", err)
	}
	if err := c.promote(); err != nil {
		t.Fatal(err)
	}
	if creates != 1 || notifications != 2 {
		t.Fatalf("rerun created=%d notified=%d", creates, notifications)
	}
	files[m.Artifact] = []byte("mismatched published bytes")
	if err := c.promote(); err == nil {
		t.Fatal("mismatched immutable release accepted")
	}
	if creates != 1 || notifications != 2 {
		t.Fatal("mismatch was overwritten or notified")
	}
}
func TestFreezeToolsUsesSelectedCoreDeclarations(t *testing.T) {
	workspace(t)
	os.MkdirAll("plugins/native-shell/sdk", 0755)
	os.WriteFile("plugins/native-shell/sdk/mise.toml", []byte("zig = \"0.17.0\"\n\"aqua:yonaskolb/XcodeGen\" = { version = \"2.46.0\" }\n"), 0644)
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != strings.Repeat("b", 40) {
			t.Fatal("not frozen to selected core source")
		}
		text := ""
		switch r.URL.Path {
		case "/repos/owner/product/contents/internal/toolchain/pins.go":
			text = "const Go = \"1.28.2\"\nconst VitePlus = \"1.4.2\"\nconst PNPM = \"13.1.0\"\n"
		case "/repos/owner/product/contents/example/mise.toml":
			text = "node = \"26.3.1\"\n"
		default:
			t.Fatal(r.URL.Path)
		}
		send(w, map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(text))})
	})
	if err := c.freezeTools(strings.Repeat("b", 40)); err != nil {
		t.Fatal(err)
	}
	var tools map[string]string
	if err := readJSON("candidate/toolchain.json", &tools); err != nil {
		t.Fatal(err)
	}
	if tools["vite_plus"] != "1.4.2" || tools["go"] != "1.28.2" || tools["node"] != "26.3.1" {
		t.Fatal(tools)
	}
	m, _, _, _ := fixture(t)
	notes, err := releaseNotes(m, "owner/product")
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{"VP_VERSION=1.4.2", "go = \"1.28.2\"", "node = \"26.3.1\"", "https://github.com/owner/product/releases/download/plugins/native-shell/v0.1.1/" + m.Artifact, "skgo plugin native-shell build --platform macos", "open native/build/macos/DerivedData/Build/Products/Debug/TinyNative.app", "export GOWORK=off"} {
		if !strings.Contains(notes, literal) {
			t.Errorf("notes lack %q", literal)
		}
	}
	if strings.Contains(notes, "@SOURCE@") || strings.Contains(notes, "1.3.0") {
		t.Fatal("notes contain unresolved or stale pins")
	}
}

func TestAutomaticCandidateUsesPublishedCoreIdentity(t *testing.T) {
	workspace(t)
	t.Setenv("GITHUB_SHA", strings.Repeat("a", 40))
	t.Setenv("GITHUB_RUN_ID", "42")
	t.Setenv("GITHUB_EVENT_NAME", "workflow_run")
	t.Setenv("GITHUB_EVENT_PATH", "event.json")
	t.Setenv("GITHUB_OUTPUT", "outputs")
	os.WriteFile("outputs", nil, 0600)
	writeJSON("event.json", map[string]any{"workflow_run": map[string]any{"id": 15}})
	identity := []byte(`{"core_version":"v0.46.0","core_source_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","core_run":15}`)
	archive, digest := zipFiles(t, map[string][]byte{"core-release.json": identity})
	os.MkdirAll("plugins/native-shell/sdk", 0755)
	os.WriteFile("plugins/native-shell/sdk/mise.toml", []byte("zig = \"0.17.0\"\nversion = \"2.46.0\"\n"), 0644)
	reserved := false
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/product/actions/runs/15":
			send(w, run{ID: 15, Path: ".github/workflows/release.yml", Conclusion: "success", Event: "workflow_dispatch"})
		case "/repos/owner/product/actions/runs/15/artifacts":
			send(w, map[string]any{"artifacts": []artifact{{ID: 50, Name: "core-release-identity", Digest: digest}}})
		case "/repos/owner/product/actions/artifacts/50/zip":
			w.Write(archive)
		case "/repos/owner/product/actions/runs/42/artifacts":
			send(w, map[string]any{"artifacts": []artifact{}})
		case "/repos/owner/product/releases/tags/v0.46.0":
			send(w, release{Tag: "v0.46.0"})
		case "/repos/owner/product/git/ref/tags/v0.46.0":
			send(w, map[string]any{"object": map[string]string{"type": "commit", "sha": strings.Repeat("b", 40)}})
		case "/repos/owner/product/releases":
			send(w, []release{})
		case "/repos/owner/product/tags":
			send(w, []map[string]string{{"name": "plugins/native-shell/v0.1.9"}, {"name": "v0.46.0"}})
		case "/repos/owner/product/git/ref/tags/plugins/native-shell/v0.1.9":
			send(w, map[string]any{"object": map[string]string{"type": "commit", "sha": strings.Repeat("a", 40)}})
		case "/repos/owner/product/git/ref/tags/plugins/native-shell/v0.1.10":
			http.NotFound(w, r)
		case "/repos/owner/product/git/tags":
			send(w, map[string]string{"sha": strings.Repeat("c", 40)})
		case "/repos/owner/product/git/refs":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["ref"] != "refs/tags/plugins/native-shell/v0.1.10" || body["sha"] != strings.Repeat("c", 40) {
				t.Fatal(body)
			}
			reserved = true
			w.WriteHeader(201)
		case "/repos/owner/product/contents/internal/toolchain/pins.go":
			send(w, map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("const Go = \"1.27.3\"\nconst VitePlus = \"1.3.0\"\nconst PNPM = \"12.14.0\"\n"))})
		case "/repos/owner/product/contents/example/mise.toml":
			send(w, map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("node = \"24.22.0\""))})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", 500)
		}
	})
	if err := c.prepare(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("outputs")
	for _, line := range []string{"core_version=v0.46.0\n", "core_source_sha=" + strings.Repeat("b", 40) + "\n", "plugin_version=v0.1.10\n", "source_sha=" + strings.Repeat("a", 40) + "\n", "reuse=false\n"} {
		if !strings.Contains(string(b), line) {
			t.Fatal(string(b))
		}
	}
	if !reserved {
		t.Fatal("version not reserved")
	}
}
func TestNonPublishingCoreRunCannotScheduleCandidate(t *testing.T) {
	workspace(t)
	t.Setenv("GITHUB_SHA", strings.Repeat("a", 40))
	t.Setenv("GITHUB_EVENT_NAME", "workflow_run")
	t.Setenv("GITHUB_EVENT_PATH", "event.json")
	writeJSON("event.json", map[string]any{"workflow_run": map[string]any{"id": 15}})
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/product/actions/runs/15":
			send(w, run{ID: 15, Path: ".github/workflows/release.yml", Conclusion: "success", Event: "workflow_dispatch"})
		case "/repos/owner/product/actions/runs/15/artifacts":
			send(w, map[string]any{"artifacts": []artifact{}})
		default:
			t.Fatal("unexpected mutation or request", r.URL)
		}
	})
	if err := c.prepare(); err == nil || !strings.Contains(err.Error(), "no published identity") {
		t.Fatal("publish=false core run accepted", err)
	}
}

func TestAuthorizationRejectsUnfinishedRunAndFailedBuild(t *testing.T) {
	m, _, archive, digest := fixture(t)
	for _, kind := range []string{"latest-running", "original-build-failed"} {
		t.Run(kind, func(t *testing.T) {
			workspace(t)
			c := candidateAPI(t, m, archive, digest, func(w http.ResponseWriter, r *http.Request) bool {
				if kind == "latest-running" && r.URL.Path == "/repos/owner/product/actions/runs/42" {
					send(w, run{ID: 42, Conclusion: ""})
					return true
				}
				if kind == "original-build-failed" && r.URL.Path == "/repos/owner/product/actions/runs/42/attempts/1/jobs" {
					send(w, map[string]any{"jobs": []map[string]string{{"name": "candidate", "conclusion": "failure"}}})
					return true
				}
				return false
			})
			if err := c.authorize(request{m, 71, digest}, "candidate"); err == nil {
				t.Fatal("unqualified run/build accepted")
			}
		})
	}
}

func TestPublishedPairRecoveryUsesPermanentReceiptAfterArchiveExpiry(t *testing.T) {
	workspace(t)
	m, files, _, _ := fixture(t)
	t.Setenv("NATIVE_RELEASE_DISPATCH_REPOSITORY", "service/qualification")
	t.Setenv("NATIVE_RELEASE_DISPATCH_TOKEN", "dispatch-token")
	var rel release
	json.Unmarshal([]byte(`{"id":91,"tag_name":"plugins/native-shell/v0.1.1","html_url":"https://github.com/owner/product/releases/tag/plugins/native-shell/v0.1.1","assets":[{"name":"candidate.json"}]}`), &rel)
	notifications := 0
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/product/releases":
			send(w, []release{rel})
		case "/repos/owner/product/git/ref/tags/plugins/native-shell/v0.1.1":
			send(w, map[string]any{"object": map[string]string{"type": "commit", "sha": m.Source}})
		case "/repos/owner/product/git/ref/tags/v0.46.0":
			send(w, map[string]any{"object": map[string]string{"type": "commit", "sha": m.CoreSource}})
		case "/repos/owner/product/releases/tags/v0.46.0":
			send(w, release{Tag: m.Core})
		case "/repos/service/qualification/dispatches":
			notifications++
			w.WriteHeader(204)
		default:
			t.Errorf("durable recovery must not need expiring Actions archive: %s", r.URL)
			http.Error(w, "expired", 404)
		}
	})
	originalGH := runGH
	t.Cleanup(func() { runGH = originalGH })
	runGH = func(args ...string) error {
		if len(args) < 2 || args[0] != "release" || args[1] != "download" {
			return fmt.Errorf("recovery attempted mutation: %v", args)
		}
		name, dir := "", ""
		for i := 2; i < len(args)-1; i++ {
			if args[i] == "--pattern" {
				name = args[i+1]
			}
			if args[i] == "--dir" {
				dir = args[i+1]
			}
		}
		return os.WriteFile(filepath.Join(dir, name), files[name], 0644)
	}
	matched, err := c.publishedPair(m.Source, m.Core, m.CoreSource)
	if err != nil || !matched || notifications != 1 {
		t.Fatalf("durable recovery matched=%v notifications=%d err=%v", matched, notifications, err)
	}
	files["toolchain.json"] = []byte(`{"go":"99.0.0"}`)
	if _, err = c.publishedPair(m.Source, m.Core, m.CoreSource); err == nil {
		t.Fatal("altered permanent toolchain accepted")
	}
	if notifications != 1 {
		t.Fatal("notified corrupt release")
	}
}

func TestUnbuiltReservationCanResumeOnlyItsOriginalRun(t *testing.T) {
	for _, owner := range []int64{42, 99} {
		t.Run(fmt.Sprint(owner), func(t *testing.T) {
			t.Setenv("GITHUB_RUN_ID", "42")
			mutations := 0
			source := strings.Repeat("a", 40)
			c := serve(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "POST":
					mutations++
					http.Error(w, "already exists", 422)
				case r.URL.Path == "/repos/owner/product/git/ref/tags/plugins/native-shell/v0.1.1":
					send(w, map[string]any{"object": map[string]string{"type": "tag", "sha": strings.Repeat("c", 40)}})
				case r.URL.Path == "/repos/owner/product/git/tags/"+strings.Repeat("c", 40):
					send(w, map[string]any{"message": fmt.Sprintf("native-shell candidate run=%d source=%s", owner, source), "object": map[string]string{"type": "commit", "sha": source}})
				case r.URL.Path == "/repos/owner/product/releases/tags/plugins/native-shell/v0.1.1":
					http.NotFound(w, r)
				case r.URL.Path == "/repos/owner/product/releases":
					send(w, []release{})
				default:
					t.Fatal(r.URL)
				}
			})
			version, err := c.reserve("v0.1.1", source)
			if owner == 42 {
				if err != nil || version != "v0.1.1" || mutations != 0 {
					t.Fatal(version, err, mutations)
				}
			} else if err == nil {
				t.Fatal("another run's reservation was adopted")
			}
		})
	}
}
func TestCoreDryRunIsSkippedBeforeNativeBuild(t *testing.T) {
	workspace(t)
	t.Setenv("GITHUB_EVENT_NAME", "workflow_run")
	t.Setenv("GITHUB_EVENT_PATH", "event.json")
	t.Setenv("GITHUB_OUTPUT", "outputs")
	os.WriteFile("outputs", nil, 0600)
	writeJSON("event.json", map[string]any{"workflow_run": map[string]int{"id": 15}})
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/product/actions/runs/15":
			send(w, run{ID: 15, Path: ".github/workflows/release.yml", Event: "workflow_dispatch", Conclusion: "success"})
		case "/repos/owner/product/actions/runs/15/jobs":
			send(w, map[string]any{"jobs": []map[string]string{{"name": "Publish GitHub Release and Homebrew cask", "conclusion": "skipped"}}})
		default:
			t.Fatal("dry run attempted candidate activity", r.URL)
		}
	})
	if err := c.eligible(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("outputs")
	if string(b) != "build=false\n" {
		t.Fatal(string(b))
	}
}

func TestDraftRecoveryUploadsOnlyMissingOriginalAssets(t *testing.T) {
	workspace(t)
	m, files, _, _ := fixture(t)
	os.Mkdir("candidate", 0755)
	for name, data := range files {
		os.WriteFile(filepath.Join("candidate", name), data, 0644)
	}
	rel := release{ID: 91, Tag: "plugins/native-shell/v0.1.1", Draft: true}
	remote := map[string][]byte{m.Artifact: files[m.Artifact]}
	updateAssets := func() {
		rel.Assets = nil
		for name := range remote {
			var a struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}
			a.Name = name
			rel.Assets = append(rel.Assets, a)
		}
	}
	updateAssets()
	originalGH := runGH
	t.Cleanup(func() { runGH = originalGH })
	uploads := 0
	failUpload := true
	runGH = func(args ...string) error {
		switch args[1] {
		case "download":
			name, dir := "", ""
			for i := 2; i < len(args)-1; i++ {
				if args[i] == "--pattern" {
					name = args[i+1]
				}
				if args[i] == "--dir" {
					dir = args[i+1]
				}
			}
			data, ok := remote[name]
			if !ok {
				return fmt.Errorf("asset missing")
			}
			return os.WriteFile(filepath.Join(dir, name), data, 0644)
		case "upload":
			path := args[len(args)-1]
			name := filepath.Base(path)
			if _, exists := remote[name]; exists {
				return fmt.Errorf("attempted overwrite of %s", name)
			}
			uploads++
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			remote[name] = data
			if failUpload {
				failUpload = false
				return fmt.Errorf("transient response loss after upload")
			}
			return nil
		default:
			return fmt.Errorf("draft staging tried unsupported mutation %v", args)
		}
	}
	c := client{repo: "owner/product"}
	if err := c.stageDraft(rel, m); err == nil {
		t.Fatal("upload failure not visible")
	}
	updateAssets()
	if err := c.stageDraft(rel, m); err != nil {
		t.Fatal(err)
	}
	if uploads != 4 {
		t.Fatalf("uploaded %d assets, want4 missing assets", uploads)
	}
	updateAssets()
	remote[m.Artifact] = []byte("conflicting staged bytes")
	before := uploads
	if err := c.stageDraft(rel, m); err == nil {
		t.Fatal("conflicting draft bytes accepted")
	}
	if uploads != before {
		t.Fatal("conflict caused an asset upload")
	}
}

func TestDraftLookupRejectsDuplicatePendingTags(t *testing.T) {
	calls := 0
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/tags/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/repos/owner/product/releases" {
			t.Fatal(r.URL)
		}
		calls++
		send(w, []release{{ID: 1, Tag: "plugins/native-shell/v0.1.1", Draft: true}, {ID: 2, Tag: "plugins/native-shell/v0.1.1", Draft: true}})
	})
	if _, err := c.releaseByTag("plugins/native-shell/v0.1.1"); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatal("ambiguous drafts accepted", err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
