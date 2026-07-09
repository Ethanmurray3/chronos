// Package mcpserver implements chronos-mcp: a Model Context Protocol server
// (JSON-RPC over stdio) that exposes a running Chronos instance to AI agents.
// It is a deliberately separate, optional binary — one person on a team can
// use it without the Chronos server or anyone else knowing it exists.
package mcpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIClient talks to the Chronos REST API.
type APIClient struct {
	Base  string // e.g. http://localhost:8787
	Token string // optional; sent as a Bearer token when set
	hc    *http.Client
}

// NewAPIClient builds a client for the Chronos server at base.
func NewAPIClient(base, token string) *APIClient {
	return &APIClient{
		Base:  strings.TrimRight(base, "/"),
		Token: token,
		hc:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *APIClient) do(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.Base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("chronos server unreachable at %s: %w", c.Base, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("chronos API %s %s: HTTP %d", method, path, res.StatusCode)
	}
	if out == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *APIClient) get(path string, out any) error        { return c.do(http.MethodGet, path, nil, out) }
func (c *APIClient) post(path string, b, out any) error    { return c.do(http.MethodPost, path, b, out) }
