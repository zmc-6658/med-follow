package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/config"
	"MedFollow/internal/route"
	"MedFollow/internal/sync"
)

func main() {
	cfg := config.MustLoad()

	if cfg.Server.Mode != "" {
		gin.SetMode(cfg.Server.Mode)
	}

	pool, err := pgxpool.New(context.Background(), cfg.Database.DSN())
	if err != nil {
		log.Fatalf("[gateway] connect database: %v", err)
	}
	defer pool.Close()

	syncer := sync.NewSyncer(pool, sync.NewYamlMockAdapter(cfg.Sync.DataDir))
	go sync.RunScheduler(context.Background(), syncer, pool, cfg.Sync.Interval)

	r := gin.Default()
	route.Setup(r, syncer, pool)

	log.Printf("[gateway] listening on %s", cfg.Server.Addr)
	if err := r.Run(cfg.Server.Addr); err != nil {
		log.Fatalf("[gateway] server error: %v", err)
	}
}
