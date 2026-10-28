package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/config"
	"MedFollow/internal/mcp"
)

// mcpserver：MCP Server 独立可执行（JSON-RPC 2.0 over stdio，无 HTTP 端口），
// 由 AI 服务以子进程拉起，caller token 经环境变量 MEDFOLLOW_MCP_TOKEN 传入
func main() {
	cfg := config.MustLoad()

	pool, err := pgxpool.New(context.Background(), cfg.Database.DSN())
	if err != nil {
		log.Fatalf("mcpserver: connect db: %v", err)
	}
	defer pool.Close()

	if err := mcp.NewServer(pool, cfg.MCP).Run(); err != nil {
		log.Fatalf("mcpserver: %v", err)
	}
}
