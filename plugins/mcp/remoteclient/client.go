// Package remoteclient calls an application's Kit remote endpoints from Go.
package remoteclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo/remoteprotocol"
)

// Client uses one application identity. Headers and HTTP may provide existing
// application authentication. Base and AppDir describe the Kit deployment.
type Client struct {
	Origin, Base, AppDir string
	Headers              http.Header
	HTTP                 *http.Client
}

// Call makes one request. Commands are never retried, and redirects are refused.
// An error after dispatch does not establish that a command had no effect.
func (c *Client) Call(ctx context.Context, id, kind string, input any) (any, error) {
	u, err := url.Parse(c.Origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("application origin must be an http(s) origin")
	}
	appDir := c.AppDir
	if appDir == "" {
		appDir = "_app"
	}
	u.Path = strings.TrimRight(c.Base, "/") + "/" + strings.Trim(appDir, "/") + "/remote/" + id
	var body io.Reader
	method := http.MethodGet
	switch kind {
	case "query":
		payload, err := remoteprotocol.QueryArgument(input)
		if err != nil {
			return nil, err
		}
		if payload != "" {
			q := u.Query()
			q.Set("payload", payload)
			u.RawQuery = q.Encode()
		}
	case "command":
		payload, err := remoteprotocol.CommandArgument(input)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(struct {
			Payload   string   `json:"payload"`
			Refreshes []string `json:"refreshes"`
		}{payload, []string{}})
		if err != nil {
			return nil, err
		}
		method, body = http.MethodPost, bytes.NewReader(data)
	default:
		return nil, fmt.Errorf("unsupported remote kind %q", kind)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header = c.Headers.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Origin", c.Origin)
		req.Header.Set("Content-Type", "application/json")
	}
	httpClient := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		httpClient = *c.HTTP
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote request failed; a command may have completed: %w", err)
	}
	defer response.Body.Close()
	const limit = 4 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("remote response exceeds 4 MiB")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote HTTP status %d", response.StatusCode)
	}
	var envelope struct {
		Type  string `json:"type"`
		Data  string `json:"data"`
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("invalid remote response: %w", err)
	}
	if envelope.Type == "error" {
		return nil, fmt.Errorf("application error %d: %s", envelope.Error.Status, envelope.Error.Message)
	}
	if envelope.Type != "result" {
		return nil, fmt.Errorf("unknown remote response type %q", envelope.Type)
	}
	value, err := devalue.Parse(envelope.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid remote data: %w", err)
	}
	object, ok := value.(*devalue.Object)
	if !ok {
		return nil, fmt.Errorf("remote result is not an envelope object")
	}
	if _, ok := object.Get("redirect"); ok {
		return nil, fmt.Errorf("application requested a redirect")
	}
	result, ok := object.Get("_")
	if !ok {
		return nil, fmt.Errorf("remote result has no operation value")
	}
	return result, nil
}
