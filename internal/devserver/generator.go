package devserver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tylergannon/skgo/devgen"
	"github.com/tylergannon/skgo/internal/gen"
)

type generatorConfig struct {
	Package     string `json:"package"`
	BeforeBuild *struct {
		Dir  string   `json:"dir"`
		Argv []string `json:"argv"`
	} `json:"beforeBuild,omitempty"`
}

type generator struct {
	root, config, goTool, work string
	output                     io.Writer
	child                      *generatorChild
	identity                   [32]byte
	// Last successful closure survives discovery failures, so edits repairing
	// broken helper code are still compared with the code actually built.
	inputs  []string
	id      uint64
	full    bool
	rebuild bool
	epoch   uint64
}

type generatorResult struct {
	response devgen.Response
	err      error
}
type generatorChild struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	replies chan generatorResult
	done    chan struct{}
}

func (c *generatorChild) close() {
	if c == nil {
		return
	}
	_ = c.in.Close()
	_ = stopProcess(c.cmd)
	<-c.done
}
func (g *generator) close() { g.child.close(); g.child = nil }

func newGenerator(o Options, work string) (*generator, error) {
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	config := o.GeneratorConfig
	if !filepath.IsAbs(config) {
		config = filepath.Join(root, config)
	}
	return &generator{root: root, config: config, goTool: o.GoTool, work: work, output: o.Output, full: true}, nil
}
func (g *generator) readConfig() (generatorConfig, []byte, error) {
	var cfg generatorConfig
	data, err := os.ReadFile(g.config)
	if err != nil {
		return cfg, nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&cfg); err != nil {
		return cfg, nil, err
	}
	var extra any
	if err = dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return cfg, nil, fmt.Errorf("generator configuration has trailing data")
	}
	if cfg.Package == "" {
		return cfg, nil, fmt.Errorf("generator package is required")
	}
	if cfg.BeforeBuild != nil && len(cfg.BeforeBuild.Argv) == 0 {
		return cfg, nil, fmt.Errorf("generator beforeBuild.argv is empty")
	}
	return cfg, data, nil
}
func (g *generator) prerequisite(ctx context.Context, cfg generatorConfig) error {
	if cfg.BeforeBuild == nil {
		return nil
	}
	argv := cfg.BeforeBuild.Argv
	command := argv[0]
	if command == "go" {
		command = g.goTool
	}
	cmd := exec.CommandContext(ctx, command, argv[1:]...)
	cmd.Dir = cfg.BeforeBuild.Dir
	if !filepath.IsAbs(cmd.Dir) {
		cmd.Dir = filepath.Join(g.root, cmd.Dir)
	}
	cmd.Stdout = g.output
	cmd.Stderr = g.output
	ownProcess(cmd)
	cmd.Cancel = func() error { return stopProcess(cmd) }
	cmd.WaitDelay = 2 * time.Second
	defer func() { _ = stopProcess(cmd) }()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generator beforeBuild: %w", err)
	}
	return nil
}

// discover uses Go's selected compiled source/dependency/embed closure, matching
// the removed source-edit runner's Go-selected input discovery. Type checking
// is deliberately left to go build; stale generated codecs may need bootstrap.
func (g *generator) discover(ctx context.Context, cfg generatorConfig) ([]string, error) {
	cmd := exec.CommandContext(ctx, g.goTool, "list", "-deps", "-compiled", "-json", cfg.Package)
	cmd.Dir = g.root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover generator inputs: %w\n%s", err, &stderr)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	paths := map[string]bool{g.config: true}
	for {
		var p struct {
			Dir                                                                                  string
			GoFiles, CompiledGoFiles, CgoFiles, EmbedFiles, IgnoredGoFiles                       []string
			SFiles, HFiles, CFiles, CXXFiles, MFiles, FFiles, SysoFiles, SwigFiles, SwigCXXFiles []string
			Module                                                                               *struct {
				Dir, GoMod string
				Replace    *struct{ Dir, GoMod string }
			}
			Error *struct{ Err string }
		}
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if p.Error != nil {
			return nil, errors.New(p.Error.Err)
		}
		// Include directory membership as well as current selected bytes: a new Go
		// file or embedded resource must invalidate the compiled helper selection.
		for _, group := range [][]string{p.GoFiles, p.CompiledGoFiles, p.CgoFiles, p.EmbedFiles, p.SFiles, p.HFiles, p.CFiles, p.CXXFiles, p.MFiles, p.FFiles, p.SysoFiles, p.SwigFiles, p.SwigCXXFiles} {
			for _, path := range group {
				if !filepath.IsAbs(path) {
					path = filepath.Join(p.Dir, path)
				}
				paths[path] = true
			}
		}
		if p.Module != nil {
			for _, mod := range []string{p.Module.GoMod, filepath.Join(p.Module.Dir, "go.sum")} {
				if mod != "" {
					paths[mod] = true
				}
			}
			if p.Module.Replace != nil {
				for _, mod := range []string{p.Module.Replace.GoMod, filepath.Join(p.Module.Replace.Dir, "go.sum")} {
					if mod != "" {
						paths[mod] = true
					}
				}
			}
		}
	}
	// Workspace selection and build configuration are compiled inputs too.
	env := exec.CommandContext(ctx, g.goTool, "env", "-json", "GOFLAGS", "GOOS", "GOARCH", "CGO_ENABLED", "GOVERSION", "GOWORK", "GOEXPERIMENT", "GOTOOLCHAIN")
	env.Dir = g.root
	data, err = env.Output()
	if err != nil {
		return nil, err
	}
	var settings map[string]string
	if err = json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	if work := settings["GOWORK"]; work != "" && work != "off" {
		paths[work] = true
		paths[work+".sum"] = true
	}
	// Persist settings in identity without changing a file in the application.
	paths["\x00"+string(data)] = true
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}
func generatorIdentity(paths []string) ([32]byte, error) {
	h := sha256.New()
	for _, path := range paths {
		fmt.Fprintf(h, "%d:%s\n", len(path), path)
		if strings.HasPrefix(path, "\x00") {
			continue
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			h.Write([]byte("absent\n"))
			continue
		}
		if err != nil {
			return [32]byte{}, err
		}
		fmt.Fprintf(h, "%d:", len(data))
		h.Write(data)
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}
func (g *generator) refresh(ctx context.Context) (bool, error) {
	cfg, _, err := g.readConfig()
	if err != nil {
		g.close()
		return false, fmt.Errorf("generator configuration: %w", err)
	}
	// A changed known input must run the targeted bootstrap before rediscovery.
	known, err := generatorIdentity(g.inputs)
	if err != nil {
		g.close()
		return false, err
	}
	changed := g.child == nil || known != g.identity
	if !changed {
		inputs, discoverErr := g.discover(ctx, cfg)
		if discoverErr != nil {
			g.close()
			return false, discoverErr
		}
		current, e := generatorIdentity(inputs)
		if e != nil {
			g.close()
			return false, e
		}
		changed = current != g.identity
	}
	if !changed {
		return false, nil
	}
	// Once refresh begins, prerequisites/build may write even without a reply.
	g.rebuild = true
	g.close()
	if err = g.prerequisite(ctx, cfg); err != nil {
		return false, err
	}
	inputs, err := g.discover(ctx, cfg)
	if err != nil {
		return false, err
	}
	identity, err := generatorIdentity(inputs)
	if err != nil {
		return false, err
	}
	binary := filepath.Join(g.work, "generator")
	cmd := exec.CommandContext(ctx, g.goTool, "build", "-o", binary, cfg.Package)
	cmd.Dir = g.root
	cmd.Stdout = g.output
	cmd.Stderr = g.output
	if err = cmd.Run(); err != nil {
		return false, fmt.Errorf("build generator: %w", err)
	}
	after, err := g.discover(ctx, cfg)
	if err != nil {
		return false, err
	}
	afterIdentity, err := generatorIdentity(after)
	if err != nil {
		return false, err
	}
	if afterIdentity != identity {
		return false, gen.ErrObsolete
	}
	child, err := startGenerator(ctx, binary, g.root, g.output)
	if err != nil {
		return false, err
	}
	g.child = child
	g.inputs = inputs
	g.identity = identity
	g.full = true
	return true, nil
}
func startGenerator(ctx context.Context, binary, root string, output io.Writer) (*generatorChild, error) {
	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = root
	cmd.Stderr = output
	ownProcess(cmd)
	cmd.Cancel = func() error { return stopProcess(cmd) }
	cmd.WaitDelay = 2 * time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c := &generatorChild{cmd: cmd, in: in, replies: make(chan generatorResult, 1), done: make(chan struct{})}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait(); _ = stopProcess(cmd); close(c.done) }()
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 4096), 16<<20)
		for scanner.Scan() {
			var response devgen.Response
			dec := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			dec.DisallowUnknownFields()
			err := dec.Decode(&response)
			if err == nil {
				var extra any
				if e := dec.Decode(&extra); !errors.Is(e, io.EOF) {
					err = fmt.Errorf("trailing stdout data")
				}
			}
			select {
			case c.replies <- generatorResult{response, err}:
			case <-c.done:
				return
			}
			if err != nil {
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		select {
		case c.replies <- generatorResult{err: err}:
		case <-c.done:
		}
	}()
	return c, nil
}
func (g *generator) pass(ctx context.Context, project *gen.Project) error {
	if _, err := g.refresh(ctx); err != nil {
		g.rebuild = true
		return err
	}
	selection := project.RawSelection()
	g.id++
	request := devgen.Request{ID: g.id, Epoch: selection.Epoch, Revision: uint64(project.Revision()), Full: g.full || selection.Full, Paths: selection.Paths}
	if err := json.NewEncoder(g.child.in).Encode(request); err != nil {
		g.rebuild = true
		g.close()
		return fmt.Errorf("generator request: %w", err)
	}
	// A lost response cannot block the coordinator forever. Cancellation kills
	// the private helper; ordinary producer errors keep its retained workers.
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	var result generatorResult
	select {
	case <-ctx.Done():
		g.rebuild = true
		g.close()
		return ctx.Err()
	case <-timer.C:
		g.rebuild = true
		g.close()
		return fmt.Errorf("generator response timed out")
	case result = <-g.child.replies:
	case <-g.child.done:
		g.rebuild = true
		g.close()
		return fmt.Errorf("generator exited before replying")
	}
	if result.err != nil || result.response.ID != request.ID {
		g.rebuild = true
		g.close()
		return fmt.Errorf("invalid generator response for request %d: %v", request.ID, result.err)
	}
	g.rebuild = g.rebuild || result.response.Generated || result.response.Error != ""
	if result.response.Error != "" {
		return fmt.Errorf("application generation: %s", result.response.Error)
	}
	// Reconcile before acknowledging: generated declarations must be captured
	// even if their filesystem events have not arrived yet.
	if err := project.Reconcile(ctx); err != nil {
		g.rebuild = true
		return err
	}
	currentIdentity, err := generatorIdentity(g.inputs)
	if err != nil {
		g.rebuild = true
		return err
	}
	if currentIdentity != g.identity {
		return gen.ErrObsolete
	}
	if !project.AcknowledgeRaw(selection.Epoch) {
		return gen.ErrObsolete
	}
	g.epoch = selection.Epoch
	g.full = false
	return nil
}
