package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/config"
	"MedFollow/internal/mcp"
	"MedFollow/internal/trace"
)

// mcpserver：MCP Server 独立可执行（JSON-RPC 2.0 over stdio，无 HTTP 端口），
// 由 AI 服务以子进程拉起，caller token 经环境变量 MEDFOLLOW_MCP_TOKEN 传入
func main() {
	cfg := config.MustLoad()

	// stdout 是 JSON-RPC 协议通道，trace 输出必须走 stderr
	trace.SetOutput(os.Stderr)
	shutdownTrace := trace.Init(trace.Config{
		ServiceName:  "medfollow-mcpserver",
		OTLPEndpoint: cfg.Trace.OtlpEndpoint,
	})
	defer func() { _ = shutdownTrace(context.Background()) }()

	pool, err := pgxpool.New(context.Background(), cfg.Database.DSN())
	if err != nil {
		log.Fatalf("mcpserver: connect db: %v", err)
	}
	defer pool.Close()

	if err := mcp.NewServer(pool, cfg.MCP).Run(); err != nil {
		log.Fatalf("mcpserver: %v", err)
	}
}
