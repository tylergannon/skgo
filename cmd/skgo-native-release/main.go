// Command skgo-native-release coordinates immutable native release candidates.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var versionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digestRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

type candidate struct {
	Run        int64  `json:"candidate_run"`
	Attempt    int    `json:"candidate_attempt"`
	Source     string `json:"source_sha"`
	CoreSource string `json:"core_source_sha"`
	Core       string `json:"core_version"`
	Plugin     string `json:"plugin_version"`
	SHA        string `json:"candidate_sha256"`
	Artifact   string `json:"artifact"`
}
type request struct {
	candidate
	ArtifactID     int64  `json:"candidate_artifact_id"`
	ArtifactDigest string `json:"candidate_artifact_digest"`
}
type artifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Digest  string `json:"digest"`
	Expired bool   `json:"expired"`
}
type run struct {
	ID         int64  `json:"id"`
	Attempt    int    `json:"run_attempt"`
	Path       string `json:"path"`
	Event      string `json:"event"`
	SHA        string `json:"head_sha"`
	Conclusion string `json:"conclusion"`
}
type release struct {
	ID         int64  `json:"id"`
	Tag        string `json:"tag_name"`
	URL        string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("GitHub API %d: %s", e.Status, e.Message) }

type client struct {
	base, repo, token string
	http              *http.Client
}

func (c client) api(method, path string, body, out any) error {
	var data io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+"/repos/"+c.repo+"/"+path, data)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return &apiError{res.StatusCode, string(b)}
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
func missing(err error) bool          { var e *apiError; return errors.As(err, &e) && e.Status == 404 }
func envInt(name string) int64        { n, _ := strconv.ParseInt(os.Getenv(name), 10, 64); return n }
func artifactName(attempt int) string { return fmt.Sprintf("native-shell-candidate-%d", attempt) }
func filename(plugin, core string) string {
	return "native-shell-" + plugin + "-skgo-" + core + "-darwin-arm64.so"
}
func (c candidate) valid() error {
	if c.Run <= 0 || c.Attempt <= 0 || !shaRE.MatchString(c.Source) || !shaRE.MatchString(c.CoreSource) || !versionRE.MatchString(c.Core) || !versionRE.MatchString(c.Plugin) || !digestRE.MatchString(c.SHA) || c.Artifact != filename(c.Plugin, c.Core) {
		return fmt.Errorf("invalid candidate identity")
	}
	return nil
}
func readJSON(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func output(key, value string) error {
	f, err := os.OpenFile(os.Getenv("GITHUB_OUTPUT"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s=%s\n", key, value)
	return err
}

var runGH = cli

func cli(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func (c client) artifacts(runID int64) ([]artifact, error) {
	var all []artifact
	for page := 1; ; page++ {
		var list struct {
			Artifacts []artifact `json:"artifacts"`
		}
		err := c.api("GET", fmt.Sprintf("actions/runs/%d/artifacts?per_page=100&page=%d", runID, page), nil, &list)
		if err != nil {
			return nil, err
		}
		all = append(all, list.Artifacts...)
		if len(list.Artifacts) < 100 {
			return all, nil
		}
	}
}
func (c client) downloadArtifact(a artifact, dir string) error {
	if a.Expired || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(a.Digest) {
		return fmt.Errorf("candidate artifact expired or lacks SHA256 digest")
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/repos/%s/actions/artifacts/%d/zip", c.base, c.repo, a.ID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("download artifact: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 512<<20))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if "sha256:"+hex.EncodeToString(sum[:]) != a.Digest {
		return fmt.Errorf("artifact archive digest mismatch")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(f.Name) != f.Name {
			return fmt.Errorf("unexpected artifact entry %q", f.Name)
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(io.LimitReader(r, 512<<20))
		r.Close()
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, f.Name), b, 0644); err != nil {
			return err
		}
	}
	return nil
}
func verifyFiles(dir string, m candidate) error {
	if err := m.valid(); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(dir, m.Artifact))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != m.SHA {
		return fmt.Errorf("plugin checksum differs from candidate manifest")
	}
	b, err = os.ReadFile(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		return err
	}
	if string(b) != m.SHA+"  "+m.Artifact+"\n" {
		return fmt.Errorf("checksum file differs from manifest")
	}
	return nil
}
func (c client) findCandidate(runID int64, dir string) (*request, error) {
	list, err := c.artifacts(runID)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	for _, a := range list {
		if !strings.HasPrefix(a.Name, "native-shell-candidate-") {
			continue
		}
		if err = c.downloadArtifact(a, dir); err != nil {
			return nil, err
		}
		var m candidate
		if err = readJSON(filepath.Join(dir, "candidate.json"), &m); err != nil {
			return nil, err
		}
		if m.Run != runID || a.Name != artifactName(m.Attempt) {
			return nil, fmt.Errorf("candidate artifact run/attempt mismatch")
		}
		if err = verifyFiles(dir, m); err != nil {
			return nil, err
		}
		return &request{m, a.ID, a.Digest}, nil
	}
	return nil, nil
}
func nextVersion(tags []string) (string, error) {
	major, minor, patch := 0, 1, -1
	found := false
	for _, tag := range tags {
		s := strings.TrimPrefix(tag, "plugins/native-shell/")
		if s == tag || !versionRE.MatchString(s) {
			continue
		}
		var a, b, d int
		if _, err := fmt.Sscanf(s, "v%d.%d.%d", &a, &b, &d); err != nil {
			return "", err
		}
		if !found || a > major || a == major && b > minor || a == major && b == minor && d > patch {
			major, minor, patch = a, b, d
			found = true
		}
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch+1), nil
}
func (c client) reserve(version, source string) (string, error) {
	if version == "" {
		var tags []string
		for page := 1; ; page++ {
			var list []struct {
				Name string `json:"name"`
			}
			if err := c.api("GET", fmt.Sprintf("tags?per_page=100&page=%d", page), nil, &list); err != nil {
				return "", err
			}
			for _, t := range list {
				tags = append(tags, t.Name)
			}
			if len(list) < 100 {
				break
			}
		}
		var err error
		version, err = nextVersion(tags)
		if err != nil {
			return "", err
		}
	}
	if !versionRE.MatchString(version) {
		return "", fmt.Errorf("plugin version must be vX.Y.Z")
	}
	err := c.api("POST", "git/refs", map[string]string{"ref": "refs/tags/plugins/native-shell/" + version, "sha": source}, nil)
	if err != nil {
		return "", fmt.Errorf("reserve immutable plugin version %s: %w", version, err)
	}
	return version, nil
}
func (c client) tagSHA(tag string) (string, error) {
	var ref struct {
		Object struct {
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"object"`
	}
	if err := c.api("GET", "git/ref/tags/"+url.PathEscape(tag), nil, &ref); err != nil {
		return "", err
	}
	for ref.Object.Type == "tag" {
		var t struct {
			Object struct {
				SHA  string `json:"sha"`
				Type string `json:"type"`
			} `json:"object"`
		}
		if err := c.api("GET", "git/tags/"+ref.Object.SHA, nil, &t); err != nil {
			return "", err
		}
		ref.Object = t.Object
	}
	if ref.Object.Type != "commit" {
		return "", fmt.Errorf("tag does not resolve to a commit")
	}
	return ref.Object.SHA, nil
}
func (c client) coreIdentity(version, source string) error {
	if !versionRE.MatchString(version) || !shaRE.MatchString(source) {
		return fmt.Errorf("invalid core identity")
	}
	var rel release
	if err := c.api("GET", "releases/tags/"+url.PathEscape(version), nil, &rel); err != nil {
		return err
	}
	if rel.Draft || rel.Prerelease || rel.Tag != version {
		return fmt.Errorf("core is not an official published release")
	}
	sha, err := c.tagSHA(version)
	if err != nil {
		return err
	}
	if sha != source {
		return fmt.Errorf("official core tag source mismatch")
	}
	return nil
}
func (c client) prepare() error {
	source := os.Getenv("GITHUB_SHA")
	if !shaRE.MatchString(source) {
		return fmt.Errorf("invalid native source SHA")
	}
	var event struct {
		WorkflowRun run `json:"workflow_run"`
		Inputs      struct {
			Core   string `json:"core_version"`
			Plugin string `json:"plugin_version"`
		} `json:"inputs"`
	}
	if err := readJSON(os.Getenv("GITHUB_EVENT_PATH"), &event); err != nil {
		return err
	}
	core, plugin, coreSource := event.Inputs.Core, event.Inputs.Plugin, ""
	if os.Getenv("GITHUB_EVENT_NAME") == "workflow_run" {
		var upstream run
		if err := c.api("GET", fmt.Sprintf("actions/runs/%d", event.WorkflowRun.ID), nil, &upstream); err != nil {
			return err
		}
		if upstream.Path != ".github/workflows/release.yml" || upstream.Conclusion != "success" || (upstream.Event != "push" && upstream.Event != "workflow_dispatch") {
			return fmt.Errorf("not a successful official core Release run")
		}
		// This artifact is emitted only by the actual publishing job, never publish=false.
		list, err := c.artifacts(upstream.ID)
		if err != nil {
			return err
		}
		found := false
		for _, a := range list {
			if a.Name == "core-release-identity" {
				if err = c.downloadArtifact(a, "core-identity"); err != nil {
					return err
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("successful core run has no published identity artifact")
		}
		var identity struct {
			Version string `json:"core_version"`
			SHA     string `json:"core_source_sha"`
			Run     int64  `json:"core_run"`
		}
		if err = readJSON("core-identity/core-release.json", &identity); err != nil {
			return err
		}
		if identity.Run != upstream.ID {
			return fmt.Errorf("core identity run mismatch")
		}
		core, coreSource = identity.Version, identity.SHA
	} else {
		if !versionRE.MatchString(core) {
			return fmt.Errorf("core version must be vX.Y.Z")
		}
		var err error
		coreSource, err = c.tagSHA(core)
		if err != nil {
			return err
		}
	}
	if err := c.coreIdentity(core, coreSource); err != nil {
		return err
	}
	existing, err := c.findCandidate(envInt("GITHUB_RUN_ID"), "candidate")
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.Source != source || existing.Core != core || existing.CoreSource != coreSource || plugin != "" && existing.Plugin != plugin {
			return fmt.Errorf("rerun inputs differ from original candidate")
		}
		return output("reuse", "true")
	}

	if plugin == "" {
		matched, err := c.publishedPair(source, core, coreSource)
		if err != nil {
			return err
		}
		if matched {
			return output("published", "true")
		}
	}
	if err = c.freezeTools(coreSource); err != nil {
		return err
	}
	plugin, err = c.reserve(plugin, source)
	if err != nil {
		return err
	}
	for k, v := range map[string]string{"core_version": core, "core_source_sha": coreSource, "plugin_version": plugin, "source_sha": source, "reuse": "false"} {
		if err = output(k, v); err != nil {
			return err
		}
	}
	return nil
}
func record() error {
	m := candidate{Run: envInt("GITHUB_RUN_ID"), Attempt: int(envInt("GITHUB_RUN_ATTEMPT")), Source: os.Getenv("GITHUB_SHA"), CoreSource: os.Getenv("CORE_SOURCE_SHA"), Core: os.Getenv("CORE_VERSION"), Plugin: os.Getenv("PLUGIN_VERSION")}
	m.Artifact = filename(m.Plugin, m.Core)
	b, err := os.ReadFile(filepath.Join("candidate", m.Artifact))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	m.SHA = hex.EncodeToString(sum[:])
	if err = m.valid(); err != nil {
		return err
	}
	if err = os.WriteFile("candidate/checksums.txt", []byte(m.SHA+"  "+m.Artifact+"\n"), 0644); err != nil {
		return err
	}
	if err = writeJSON("candidate/candidate.json", m); err != nil {
		return err
	}
	notes, err := releaseNotes(m, os.Getenv("GITHUB_REPOSITORY"))
	if err != nil {
		return err
	}
	return os.WriteFile("candidate/notes.md", []byte(notes), 0644)
}
func dispatchConfig() (string, string, error) {
	repo, token := os.Getenv("NATIVE_RELEASE_DISPATCH_REPOSITORY"), os.Getenv("NATIVE_RELEASE_DISPATCH_TOKEN")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repo) || token == "" {
		return "", "", fmt.Errorf("configure NATIVE_RELEASE_DISPATCH_REPOSITORY and NATIVE_RELEASE_DISPATCH_TOKEN secrets")
	}
	return repo, token, nil
}
func (c client) dispatch(event string, payload any) error {
	repo, token, err := dispatchConfig()
	if err != nil {
		return err
	}
	c.repo, c.token = repo, token
	return c.api("POST", "dispatches", map[string]any{"event_type": event, "client_payload": payload}, nil)
}
func (c client) notify() error {
	r, err := c.findCandidate(envInt("GITHUB_RUN_ID"), "candidate")
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("candidate artifact missing")
	}
	return c.dispatch("native-plugin-candidate", r)
}
func (c client) authorize(r request, dir string) error {
	if err := r.valid(); err != nil {
		return err
	}
	if r.ArtifactID <= 0 || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(r.ArtifactDigest) {
		return fmt.Errorf("invalid original artifact identity")
	}
	var original run
	if err := c.api("GET", fmt.Sprintf("actions/runs/%d/attempts/%d", r.Run, r.Attempt), nil, &original); err != nil {
		return err
	}
	if original.ID != r.Run || original.Attempt != r.Attempt || original.Path != ".github/workflows/native-plugin.yml" || original.SHA != r.Source || (original.Event != "workflow_dispatch" && original.Event != "workflow_run") {
		return fmt.Errorf("candidate run attempt is not an authorized successful native build")
	}

	var latest run
	if err := c.api("GET", fmt.Sprintf("actions/runs/%d", r.Run), nil, &latest); err != nil {
		return err
	}
	if latest.Conclusion != "success" {
		return fmt.Errorf("candidate run has not completed successfully")
	}
	var jobs struct {
		Jobs []struct {
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"jobs"`
	}
	if err := c.api("GET", fmt.Sprintf("actions/runs/%d/attempts/%d/jobs?per_page=100", r.Run, r.Attempt), nil, &jobs); err != nil {
		return err
	}
	built := false
	for _, j := range jobs.Jobs {
		if j.Name == "candidate" && j.Conclusion == "success" {
			built = true
		}
	}
	if !built {
		return fmt.Errorf("original candidate build job did not succeed")
	}
	list, err := c.artifacts(r.Run)
	if err != nil {
		return err
	}
	var selected *artifact
	for _, a := range list {
		if a.ID == r.ArtifactID {
			copy := a
			selected = &copy
			break
		}
	}
	if selected == nil || selected.Name != artifactName(r.Attempt) || selected.Digest != r.ArtifactDigest {
		return fmt.Errorf("original candidate artifact does not match request")
	}
	if err = c.downloadArtifact(*selected, dir); err != nil {
		return err
	}
	var actual candidate
	if err = readJSON(filepath.Join(dir, "candidate.json"), &actual); err != nil {
		return err
	}
	if actual != r.candidate {
		return fmt.Errorf("promotion differs from authoritative candidate manifest")
	}
	if err = verifyFiles(dir, actual); err != nil {
		return err
	}
	source, err := c.tagSHA("plugins/native-shell/" + r.Plugin)
	if err != nil {
		return err
	}
	if source != r.Source {
		return fmt.Errorf("reserved plugin tag source mismatch")
	}
	return c.coreIdentity(r.Core, r.CoreSource)
}
func (c client) verifyPublished(rel release, m candidate, dir string) error {
	if rel.Draft || rel.Prerelease || rel.Tag != "plugins/native-shell/"+m.Plugin {
		return fmt.Errorf("existing release identity mismatch")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, name := range []string{m.Artifact, "checksums.txt", "candidate.json", "toolchain.json"} {
		if err := runGH("release", "download", rel.Tag, "--repo", c.repo, "--pattern", name, "--dir", dir, "--clobber"); err != nil {
			return err
		}
	}
	var actual candidate
	if err := readJSON(filepath.Join(dir, "candidate.json"), &actual); err != nil {
		return err
	}
	if actual != m {
		return fmt.Errorf("published release differs from original candidate")
	}
	return verifyFiles(dir, m)
}
func (c client) promote() error {
	if _, _, err := dispatchConfig(); err != nil {
		return err
	}
	var event struct {
		Payload request `json:"client_payload"`
	}
	if err := readJSON(os.Getenv("GITHUB_EVENT_PATH"), &event); err != nil {
		return err
	}
	r := event.Payload
	if err := c.authorize(r, "candidate"); err != nil {
		return err
	}
	tag := "plugins/native-shell/" + r.Plugin
	var rel release
	err := c.api("GET", "releases/tags/"+url.PathEscape(tag), nil, &rel)
	if missing(err) {
		if err = runGH("release", "create", tag, "--repo", c.repo, "--verify-tag", "--latest=false", "--title", "native-shell "+r.Plugin+" for skgo "+r.Core, "--notes-file", "candidate/notes.md", filepath.Join("candidate", r.Artifact), "candidate/checksums.txt", "candidate/candidate.json", "candidate/toolchain.json"); err != nil {
			return err
		}
		if err = c.api("GET", "releases/tags/"+url.PathEscape(tag), nil, &rel); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err = c.verifyPublished(rel, r.candidate, "published"); err != nil {
		return err
	}
	if err = compareFile("candidate/toolchain.json", "published/toolchain.json"); err != nil {
		return err
	}
	return c.publishedNotification(r, rel)
}
func (c client) publishedNotification(r request, rel release) error {
	return c.dispatch("native-plugin-published", struct {
		Candidate request `json:"candidate"`
		ReleaseID int64   `json:"release_id"`
		Tag       string  `json:"release_tag"`
		URL       string  `json:"release_url"`
	}{r, rel.ID, rel.Tag, rel.URL})
}
func execute() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: skgo-native-release prepare|record|notify|promote|check-config")
	}
	c := client{os.Getenv("GITHUB_API_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GH_TOKEN"), http.DefaultClient}
	if c.base == "" {
		c.base = "https://api.github.com"
	}
	switch os.Args[1] {
	case "prepare":
		if _, _, err := dispatchConfig(); err != nil {
			return err
		}
		return c.prepare()
	case "record":
		return record()
	case "notify":
		return c.notify()
	case "promote":
		return c.promote()
	case "check-config":
		_, _, err := dispatchConfig()
		return err
	}
	return fmt.Errorf("unknown command")
}
func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (c client) sourceFile(path, source string) (string, error) {
	var f struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := c.api("GET", "contents/"+path+"?ref="+source, nil, &f); err != nil {
		return "", err
	}
	if f.Encoding != "base64" {
		return "", fmt.Errorf("unexpected source encoding")
	}
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(f.Content, "\n", ""))
	return string(b), err
}
func (c client) freezeTools(coreSource string) error {
	pins, err := c.sourceFile("internal/toolchain/pins.go", coreSource)
	if err != nil {
		return err
	}
	mise, err := c.sourceFile("example/mise.toml", coreSource)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for key, name := range map[string]string{"go": "Go", "vite_plus": "VitePlus", "pnpm": "PNPM"} {
		m := regexp.MustCompile(`const ` + name + ` = "([0-9.]+)"`).FindStringSubmatch(pins)
		if len(m) != 2 {
			return fmt.Errorf("core has no qualified %s pin", name)
		}
		values[key] = m[1]
	}
	node := regexp.MustCompile(`node = "([0-9.]+)"`).FindStringSubmatch(mise)
	if len(node) != 2 {
		return fmt.Errorf("core has no exact Node pin")
	}
	values["node"] = node[1]
	values["zig"], err = pin("plugins/native-shell/sdk/mise.toml", `zig = "([0-9.]+)"`)
	if err != nil {
		return err
	}
	values["xcodegen"], err = pin("plugins/native-shell/sdk/mise.toml", `version = "([0-9.]+)"`)
	if err != nil {
		return err
	}
	if err = os.MkdirAll("candidate", 0755); err != nil {
		return err
	}
	return writeJSON("candidate/toolchain.json", values)
}

func (c client) publishedPair(source, core, coreSource string) (bool, error) {
	for page := 1; ; page++ {
		var releases []release
		if err := c.api("GET", fmt.Sprintf("releases?per_page=100&page=%d", page), nil, &releases); err != nil {
			return false, err
		}
		for _, rel := range releases {
			if !strings.HasPrefix(rel.Tag, "plugins/native-shell/") {
				continue
			}
			hasManifest := false
			for _, a := range rel.Assets {
				if a.Name == "candidate.json" {
					hasManifest = true
				}
			}
			if !hasManifest {
				continue
			}
			dir, err := os.MkdirTemp("", "native-published-")
			if err != nil {
				return false, err
			}
			err = runGH("release", "download", rel.Tag, "--repo", c.repo, "--pattern", "candidate.json", "--dir", dir)
			if err != nil {
				os.RemoveAll(dir)
				return false, err
			}
			var m candidate
			err = readJSON(filepath.Join(dir, "candidate.json"), &m)
			if err != nil {
				os.RemoveAll(dir)
				return false, err
			}
			if m.Source == source && m.Core == core {
				if m.CoreSource != coreSource {
					os.RemoveAll(dir)
					return false, fmt.Errorf("published pairing core source mismatch")
				}
				err = c.verifyPublished(rel, m, dir)
				defer os.RemoveAll(dir)
				if err != nil {
					return false, err
				}
				tagSource, err := c.tagSHA(rel.Tag)
				if err != nil {
					return false, err
				}
				if tagSource != source {
					return false, fmt.Errorf("published pairing tag mismatch")
				}

				original, err := c.findCandidate(m.Run, "published-original")
				if err != nil {
					return false, err
				}
				if original == nil || original.candidate != m {
					return false, fmt.Errorf("published pairing has no matching authoritative candidate")
				}
				if err = c.authorize(*original, "published-original"); err != nil {
					return false, err
				}
				if err = compareFile("published-original/toolchain.json", filepath.Join(dir, "toolchain.json")); err != nil {
					return false, err
				}
				if err = c.publishedNotification(*original, rel); err != nil {
					return false, err
				}
				fmt.Printf("Already published exact pairing: %s for %s at %s\n", rel.Tag, core, source)
				return true, nil
			}
			os.RemoveAll(dir)
		}
		if len(releases) < 100 {
			return false, nil
		}
	}
}

func compareFile(a, b string) error {
	first, err := os.ReadFile(a)
	if err != nil {
		return err
	}
	second, err := os.ReadFile(b)
	if err != nil {
		return err
	}
	if !bytes.Equal(first, second) {
		return fmt.Errorf("published %s differs from original candidate", filepath.Base(a))
	}
	return nil
}
