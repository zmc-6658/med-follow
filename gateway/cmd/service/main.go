package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"MedFollow/internal/config"
	"MedFollow/internal/route"
)

func main() {
	cfg := config.MustLoad()

	if cfg.Server.Mode != "" {
		gin.SetMode(cfg.Server.Mode)
	}

	r := gin.Default()
	route.Setup(r)

	log.Printf("[gateway] listening on %s", cfg.Server.Addr)
	if err := r.Run(cfg.Server.Addr); err != nil {
		log.Fatalf("[gateway] server error: %v", err)
	}
}
