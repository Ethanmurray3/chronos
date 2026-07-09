package mcpserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
)

// The MCP stdio transport is newline-delimited JSON-RPC 2.0. We implement the
// small slice of the protocol a tools-only server needs: initialize, ping,
// tools/list, tools/call. Everything else gets a polite method-not-found.

const (
	fallbackProtocolVersion = "2025-06-18"
	serverVersion           = "0.4.0"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"` // absent on notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server binds the tool table to a Chronos API client and a stdio pair.
type Server struct {
	api   *APIClient
	tools []Tool
	out   *json.Encoder
	mu    sync.Mutex // one writer at a time on stdout
}

// New builds a Server over the given API client.
func New(api *APIClient) *Server {
	return &Server{api: api, tools: toolTable()}
}

// Run processes messages from in until EOF. Diagnostics go to the process
// stderr (never stdout — that would corrupt the protocol stream).
func (s *Server) Run(in io.Reader, out io.Writer) error {
	s.out = json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("bad JSON-RPC frame: %v", err)
			continue
		}
		s.dispatch(req)
	}
	return sc.Err()
}

func (s *Server) dispatch(req rpcRequest) {
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := p.ProtocolVersion
		if version == "" {
			version = fallbackProtocolVersion
		}
		s.reply(req.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "chronos-mcp",
				"version": serverVersion,
			},
		})

	case "ping":
		s.reply(req.ID, map[string]any{})

	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": s.tools})

	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			s.replyErr(req.ID, -32602, "invalid tools/call params: "+err.Error())
			return
		}
		s.reply(req.ID, s.callTool(p.Name, p.Arguments))

	default:
		if isNotification {
			return // notifications (e.g. notifications/initialized) need no reply
		}
		s.replyErr(req.ID, -32601, fmt.Sprintf("method %q not supported", req.Method))
	}
}

// callTool runs a tool and wraps the outcome in an MCP tool result. Tool
// failures are reported in-band (isError: true), not as JSON-RPC errors.
func (s *Server) callTool(name string, args map[string]any) map[string]any {
	for _, t := range s.tools {
		if t.Name != name {
			continue
		}
		text, err := t.handler(s.api, args)
		if err != nil {
			return toolResult("Error: "+err.Error(), true)
		}
		return toolResult(text, false)
	}
	return toolResult(fmt.Sprintf("unknown tool %q", name), true)
}

func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) replyErr(id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		return
	}
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: code, Message: msg}})
}

func (s *Server) send(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.out.Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
