package dev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type viteControl struct {
	Token string `json:"token"`
	Base  string `json:"base"`
}

func (s *Server) control() (viteControl, error) {
	var control viteControl
	data, err := os.ReadFile(filepath.Join(s.cfg.Web, ".svelte-kit", "skgo-dev-supervisor.json"))
	if err != nil {
		return control, err
	}
	if err = json.Unmarshal(data, &control); err != nil {
		return control, err
	}
	if control.Token == "" || !strings.HasPrefix(control.Base, "/") || strings.HasPrefix(control.Base, "//") {
		return control, fmt.Errorf("invalid Vite supervisor descriptor")
	}
	return control, nil
}

// Queue only the latest settled status. Vite may still be starting when the
// first Go build fails; delivery retries independently of generation/requests.
func (s *Server) reload(report string) {
	if s.cfg.Vite == nil {
		return
	}
	select {
	case s.reloads <- report:
	default:
		select {
		case <-s.reloads:
		default:
		}
		select {
		case s.reloads <- report:
		default:
		}
	}
}

func (s *Server) reloadLoop(ctx context.Context) {
	client := &http.Client{Timeout: time.Second}
	for {
		var report string
		select {
		case <-ctx.Done():
			return
		case report = <-s.reloads:
		}
		for ctx.Err() == nil {
			select {
			case report = <-s.reloads:
			default:
			}
			if err := s.sendReload(ctx, client, report); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return
			case report = <-s.reloads:
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

func (s *Server) sendReload(ctx context.Context, client *http.Client, report string) error {
	control, err := s.control()
	if err != nil {
		return err
	}
	target := *s.cfg.Vite
	target.Path = strings.TrimSuffix(control.Base, "/") + "/__skgo_dev/supervisor"
	target.RawQuery = ""
	body, _ := json.Marshal(struct {
		Diagnostic string `json:"diagnostic"`
	}{report})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Skgo-Dev-Token", control.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Vite supervisor status: %s", res.Status)
	}
	return nil
}

// Vite's own client must remain reachable on the diagnostic page, including
// while there is no application process to proxy it. Other paths still see the
// Go diagnostic rather than falling through to Kit's JavaScript server.
func (s *Server) viteClientPath() string {
	base := "/"
	if control, err := s.control(); err == nil {
		base = control.Base
	}
	return strings.TrimSuffix(base, "/") + "/@vite/"
}
