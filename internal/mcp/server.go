// Package mcp implements a minimal Model Context Protocol (MCP) server over
// stdio using JSON-RPC 2.0.
//
// Protocol summary (MCP 1.0, stdio transport):
//   - Messages are newline-delimited JSON (NDJSON)
//   - Request:      {"jsonrpc":"2.0","id":N,"method":"...","params":{...}}
//   - Response:     {"jsonrpc":"2.0","id":N,"result":{...}}
//   - Error:        {"jsonrpc":"2.0","id":N,"error":{"code":N,"message":"..."}}
//   - Notification: {"jsonrpc":"2.0","method":"...","params":{...}}  (no id)
//
// Handshake:
//  1. Client → initialize
//  2. Server → InitializeResult
//  3. Client → initialized  (notification, no response needed)
//  4. Client → tools/list, tools/call, ...
//
// Exposed tools:
//   - loar_explain: retrieve evidence for a question from the current project
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/retrieval"
	"github.com/balpal4495/loar/internal/store"
)

// Server is a stateful MCP server that wraps a Loar store and retrieval engine.
type Server struct {
	store     store.Store
	projectID string
	projectName string
	in        io.Reader
	out       io.Writer
}

// New creates a Server. projectID is the UUID of the project to query.
func New(s store.Store, projectID, projectName string, in io.Reader, out io.Writer) *Server {
	return &Server{
		store:       s,
		projectID:   projectID,
		projectName: projectName,
		in:          in,
		out:         out,
	}
}

// Serve reads JSON-RPC requests from in line-by-line and writes responses to
// out. Blocks until in is closed (i.e. the client disconnects).
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		s.handleLine(ctx, line)
	}
	return scanner.Err()
}

// --- JSON-RPC types ---

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handleLine(ctx context.Context, line string) {
	var req request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		s.writeError(nil, -32700, "parse error")
		return
	}

	// Notifications have no id — no response required.
	if req.ID == nil || string(req.ID) == "null" {
		return
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	default:
		s.writeError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req request) {
	s.writeResult(req.ID, map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "loar",
			"version": "0.1",
		},
	})
}

func (s *Server) handleToolsList(req request) {
	s.writeResult(req.ID, map[string]any{
		"tools": []any{
			map[string]any{
				"name": "loar_explain",
				"description": fmt.Sprintf(
					"Retrieve evidence, entities, relationships, and contradictions for a question from the Loar knowledge store (project: %q). "+
						"Returns a structured context package. Synthesize the observations into an answer for the user.",
					s.projectName,
				),
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{
							"type":        "string",
							"description": "The natural-language question to investigate",
						},
					},
					"required": []string{"question"},
				},
			},
		},
	})
}

func (s *Server) handleToolsCall(ctx context.Context, req request) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params")
		return
	}

	switch params.Name {
	case "loar_explain":
		s.callExplain(ctx, req, params.Arguments)
	default:
		s.writeError(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	}
}

func (s *Server) callExplain(ctx context.Context, req request, rawArgs json.RawMessage) {
	var args struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Question == "" {
		s.writeError(req.ID, -32602, "loar_explain requires a non-empty 'question' argument")
		return
	}

	engine := retrieval.New(s.store)
	pkg, err := engine.Query(ctx, s.projectID, args.Question)
	if err != nil {
		s.writeError(req.ID, -32603, fmt.Sprintf("retrieval error: %s", err.Error()))
		return
	}

	// Serialize the context package to JSON and return it as a text content item.
	text, err := marshalContextPackage(pkg)
	if err != nil {
		s.writeError(req.ID, -32603, "failed to marshal context package")
		return
	}

	s.writeResult(req.ID, map[string]any{
		"content": []any{
			map[string]any{
				"type": "text",
				"text": text,
			},
		},
	})
}

// marshalContextPackage serialises the context package to an indented JSON
// string suitable for consumption by an AI agent.
func marshalContextPackage(pkg *domain.ContextPackage) (string, error) {
	out := map[string]any{
		"query":      pkg.Query,
		"confidence": pkg.Confidence,
		"summary":    pkg.Summary,
	}

	// Entities — name + type only (IDs are internal).
	entities := make([]map[string]any, 0, len(pkg.Entities))
	for _, e := range pkg.Entities {
		entities = append(entities, map[string]any{
			"name": e.CanonicalName,
			"type": e.Type,
		})
	}
	out["entities"] = entities

	// Observations — full content, occurred_at, source_id.
	obs := make([]map[string]any, 0, len(pkg.Observations))
	for _, o := range pkg.Observations {
		item := map[string]any{
			"content":   o.Content,
			"source_id": o.SourceID,
		}
		if o.Temporal.OccurredAt != nil {
			item["occurred_at"] = o.Temporal.OccurredAt.Format("2006-01-02")
		}
		if o.Temporal.ResolvedAt != nil {
			item["resolved_at"] = o.Temporal.ResolvedAt.Format("2006-01-02")
		}
		obs = append(obs, item)
	}
	out["observations"] = obs

	// Contradictions — summary only.
	contradictions := make([]string, 0, len(pkg.Contradictions))
	for _, c := range pkg.Contradictions {
		contradictions = append(contradictions, c.Summary)
	}
	out["contradictions"] = contradictions

	// Date range from timeline.
	if len(pkg.Timeline) > 0 {
		var earliest, latest string
		for _, ev := range pkg.Timeline {
			var ds string
			if ev.Temporal.OccurredAt != nil {
				ds = ev.Temporal.OccurredAt.Format("2006-01-02")
			} else if ev.Temporal.ObservedAt != nil {
				ds = ev.Temporal.ObservedAt.Format("2006-01-02")
			}
			if ds == "" {
				continue
			}
			if earliest == "" || ds < earliest {
				earliest = ds
			}
			if latest == "" || ds > latest {
				latest = ds
			}
		}
		if earliest != "" {
			out["date_range"] = map[string]string{"earliest": earliest, "latest": latest}
		}
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Server) writeResult(id json.RawMessage, result any) {
	s.write(response{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeError(id json.RawMessage, code int, msg string) {
	s.write(response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

func (s *Server) write(r response) {
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	fmt.Fprintf(s.out, "%s\n", b)
}
