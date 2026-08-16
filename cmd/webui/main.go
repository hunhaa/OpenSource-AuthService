//go:build legacy_webui

package main

import (
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"context"
	"time"

	"github.com/gin-gonic/gin"
	webui "github.com/Yeah114/FunAuth/modules/webui"
)

func main() {
	gin.DefaultWriter = io.MultiWriter(os.Stdout)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stdout)
	log.SetOutput(os.Stdout)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})

	api := r.Group("/api")
	webui.RegisterRoutes(api, r)

	addr := os.Getenv("FUNAUTH_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	if runtime.GOOS == "linux" {
		if p, ok := parsePort(addr); ok {
			log.Printf("[port] try free port %d before binding", p)
			freePortLinux(p)
		}
	}

	log.Printf("[server] Bunker Web binding address: %s", addr)
	log.Printf("[server] 控制台地址: http://localhost%s/ui/", prettyAddr(addr))
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func prettyAddr(a string) string {
	if strings.HasPrefix(a, ":") {
		return a
	}
	return a
}

func parsePort(addr string) (int, bool) {
	a := strings.TrimSpace(addr)
	if a == "" {
		return 0, false
	}
	if after, ok := strings.CutPrefix(a, ":"); ok {
		a = after
	} else if strings.Contains(a, ":") {
		idx := strings.LastIndex(a, ":")
		if idx >= 0 && idx+1 < len(a) {
			a = a[idx+1:]
		}
	}
	v, err := strconv.Atoi(a)
	if err != nil || v <= 0 || v > 65535 {
		return 0, false
	}
	return v, true
}

func freePortLinux(port int) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pids := findPidsWithSS(ctx, port)
	if len(pids) == 0 {
		pids = findPidsWithLsof(ctx, port)
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

func findPidsWithSS(ctx context.Context, port int) []int {
	c, err := lookupCmd(ctx, "ss")
	if err != nil {
		return nil
	}
	out, err := runCmd(ctx, c, "-ltnp")
	if err != nil {
		return nil
	}
	return parsePidFromNetPortOutput(out, port, "ss")
}

func findPidsWithLsof(ctx context.Context, port int) []int {
	c, err := lookupCmd(ctx, "lsof")
	if err != nil {
		return nil
	}
	out, err := runCmd(ctx, c, "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-t")
	if err != nil {
		return nil
	}
	return parsePidFromLsofT(out)
}
