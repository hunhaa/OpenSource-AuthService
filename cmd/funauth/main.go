//go:build legacy && !windows

package main

import (
	"log"
	"os"

	"github.com/Yeah114/FunAuth/internal/router"
	"github.com/Yeah114/FunAuth/internal/db"
	unified "github.com/Yeah114/FunAuth/internal/unified"
)

func main() {
	log.SetOutput(os.Stdout)

	if _, err := db.EnsureConfigInteractive(); err != nil {
		log.Fatalf("初始化配置失败: %v", err)
	}

	if err := db.InitDBWithOptions(db.InitOptions{}); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	unified.InitProxyPoolThenCom4399()

	r := router.NewRouter()

	addr := os.Getenv("FUNAUTH_ADDR")
	if addr == "" {
		addr = ":8090"
	}

	log.Printf("[server] binding address: %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
