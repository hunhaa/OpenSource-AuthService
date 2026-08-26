// Package main 是 FunAuth 用户中心的独立二进制入口。
//
// 启动行为：
//  1. 检测 config.json / FUNAUTH_MYSQL_DSN 是否已配置可用数据库。
//  2. 未配置 → 启动 Setup 向导服务（监听 :8090），用户访问 http://host:8090/ 完成首次配置；
//     完成后写入 config.json 并初始化数据库、管理员账号、默认身份组、网站信息。
//  3. 已配置 → 启动正式用户中心服务（API: /api/usercenter/*；SPA: /uc/）。
//
// 与主程序 funauth-linux-amd64 的关系：
//   - funauth 主程序负责 Bunker 控制台（/ui/）与 Phoenix 验证端点（/api/phoenix/* 等）；
//   - usercenter 独立二进制只负责用户中心；
//   - 两者共享同一个 MySQL 数据库（同一份 config.json）。
//   - 部署时先启动 funauth 主程序，再启动 usercenter。
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/Yeah114/FunAuth/internal/db"
	uc "github.com/Yeah114/FunAuth/modules/usercenter"
)

func main() {
	gin.DefaultWriter = io.MultiWriter(os.Stdout)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stdout)
	log.SetOutput(os.Stdout)

	addr := strings.TrimSpace(os.Getenv("FUNAUTH_ADDR"))
	if addr == "" {
		addr = os.Getenv("USERCENTER_ADDR")
	}
	if addr == "" {
		addr = ":8090"
	}

	// 检测配置是否可用
	if !configReady() {
		log.Printf("[usercenter] 配置未就绪，启动 Setup 向导服务：http://localhost%s/", prettyAddr(addr))
		if err := runSetupServer(addr); err != nil {
			log.Fatalf("[usercenter] setup 服务异常: %v", err)
		}
		return
	}

	// 配置就绪 → 启动正式服务
	if err := db.InitDBWithOptions(db.InitOptions{StartCom4399AccountPool: false}); err != nil {
		log.Fatalf("[usercenter] 初始化数据库失败: %v", err)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})
	r.Use(cors.New(cors.Config{
		AllowOriginFunc:  func(string) bool { return true },
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Session-Token"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	api := r.Group("/api")
	uc.RegisterRoutes(api, r)

	// 根路径重定向到用户中心 SPA
	r.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/uc/")
	})

	log.Printf("[usercenter] 正式服务启动，监听 %s", addr)
	log.Printf("[usercenter] 用户中心: http://localhost%s/uc/", prettyAddr(addr))
	log.Printf("[usercenter] 健康检查: http://localhost%s/api/usercenter/health", prettyAddr(addr))
	if err := r.Run(addr); err != nil {
		log.Fatalf("[usercenter] 服务退出: %v", err)
	}
}

// configReady 判断数据库配置是否就绪
// 优先级：FUNAUTH_MYSQL_DSN 环境变量 > config.json
// 都没有，或仍是占位"密码"，则视为未就绪
func configReady() bool {
	if env := strings.TrimSpace(os.Getenv("FUNAUTH_MYSQL_DSN")); env != "" {
		return true
	}
	cfg, err := db.LoadConfig()
	if err != nil || cfg == nil {
		return false
	}
	dsn := db.ResolveDSN(cfg)
	// 含占位密码 / 默认占位用户 → 视为未配置
	if strings.Contains(dsn, ":密码@") {
		return false
	}
	if cfg.MySQL.Host == "" && cfg.MySQL.Username == "" && cfg.MySQL.DSN == "" {
		return false
	}
	return true
}

func prettyAddr(a string) string {
	if strings.HasPrefix(a, ":") {
		return a
	}
	return a
}

// runSetupServer 启动 Setup 向导服务
func runSetupServer(addr string) error {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})

	// 挂载 setup 路由
	uc.RegisterSetupRoutes(r)

	r.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/setup")
	})

	fmt.Printf(`
   ___              _   _         _   _
  / __| |_ _ ___ __| |_| |_  __ _| |_| |_
  \__ \  _| '_/ _\  _|  _| |/ _. |  _|   |
  |___/\__| \__\___\__|\__|_|\__,_|__|_|_|

       FunAuth 用户中心 - 首次配置向导

  访问 http://localhost%s/ 完成首次配置。
  完成后会自动初始化数据库、创建管理员账号。
  之后请重启本程序进入正常服务模式。

`, prettyAddr(addr))

	if err := r.Run(addr); err != nil {
		return fmt.Errorf("setup server: %w", err)
	}
	return nil
}
