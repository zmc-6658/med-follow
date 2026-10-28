package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/config"
)

// JSON-RPC 2.0 over stdio 的最小 MCP Server：
// stdin 每行一条 JSON 请求，响应单行 JSON 写回 stdout，日志走 stderr（stdout 只跑协议）。

const (
	codeParseError     = -32700
	codeUnauthorized   = -32001
	codeMethodNotFound = -32601 // 语义复用：method not found / 白名单 permission denied
	codeInvalidParams  = -32602
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Server struct {
	pool   *pgxpool.Pool
	cfg    config.MCPConfig
	caller string // initialize 鉴权通过后的 caller 身份（config.callers 的 key）
}

func NewServer(pool *pgxpool.Pool, cfg config.MCPConfig) *Server {
	return &Server{pool: pool, cfg: cfg}
}

// Run 从 stdin 逐行读请求直到 EOF（客户端关闭子进程即退出）
func (s *Server) Run() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.write(Response{Error: &RPCError{Code: codeParseError, Message: "parse error"}})
			continue
		}
		s.write(*s.dispatch(&req))
	}
	return scanner.Err()
}

func (s *Server) dispatch(req *Request) *Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize()
	case "tools/list":
		if s.caller == "" {
			return errResp(codeUnauthorized, "not initialized")
		}
		return &Response{Result: map[string]any{"tools": toolDefs()}}
	case "tools/call":
		if s.caller == "" {
			return errResp(codeUnauthorized, "not initialized")
		}
		return s.handleToolsCall(req.Params)
	default:
		return errResp(codeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// handleInitialize 最小鉴权钩子：caller token 由客户端经环境变量 MEDFOLLOW_MCP_TOKEN 传入，
// 与 config.callers 中各 caller 的 token 比对，命中即确认 caller 身份
func (s *Server) handleInitialize() *Response {
	token := os.Getenv("MEDFOLLOW_MCP_TOKEN")
	for name, c := range s.cfg.Callers {
		if c.Token != "" && c.Token == token {
			s.caller = name
			log.Printf("[mcp] caller=%s method=initialize ok", name)
			return &Response{Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "medfollow-gateway", "version": "0.1.0"},
			}}
		}
	}
	log.Printf("[mcp] method=initialize rejected: invalid token")
	return errResp(codeUnauthorized, "invalid token")
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolsCall(params json.RawMessage) *Response {
	var p callParams
	if err := json.Unmarshal(params, &p); err != nil {
		return errResp(codeInvalidParams, "invalid params")
	}

	caller, ok := s.cfg.Callers[s.caller]
	if !ok || !allowed(caller.Allow, p.Name) {
		log.Printf("[mcp] caller=%s tool=%s denied", s.caller, p.Name)
		return errResp(codeMethodNotFound,
			fmt.Sprintf("permission denied: tool %s not allowed for caller %s", p.Name, s.caller))
	}

	start := time.Now()
	result, err := s.execTool(p.Name, p.Arguments)
	duration := time.Since(start)
	if err != nil {
		log.Printf("[mcp] caller=%s tool=%s duration=%s error=%v", s.caller, p.Name, duration, err)
		return &Response{Result: toolErrorResult(err.Error())}
	}
	// 每次工具调用留痕：caller + 工具名 + 耗时
	log.Printf("[mcp] caller=%s tool=%s duration=%s", s.caller, p.Name, duration)
	return &Response{Result: toolResult(result)}
}

func (s *Server) write(resp Response) {
	resp.JSONRPC = "2.0"
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[mcp] marshal response: %v", err)
		return
	}
	os.Stdout.Write(append(data, '\n'))
}

func errResp(code int, msg string) *Response {
	return &Response{Error: &RPCError{Code: code, Message: msg}}
}

// toolResult / toolErrorResult 按 MCP 约定把工具结果包进 content[0].text（JSON 字符串）
func toolResult(result any) map[string]any {
	data, _ := json.Marshal(result)
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": string(data)}},
		"isError": false,
	}
}

func toolErrorResult(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": msg}},
		"isError": true,
	}
}

func allowed(list []string, name string) bool {
	for _, v := range list {
		if v == name {
			return true
		}
	}
	return false
}
