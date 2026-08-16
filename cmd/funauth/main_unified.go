package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Yeah114/FunAuth/cmd/funauth/internal/handlers"
	com4399cli "github.com/Yeah114/FunAuth/internal/cli"
	"github.com/Yeah114/FunAuth/internal/db"
	"github.com/Yeah114/FunAuth/internal/portkit"
	"github.com/gin-gonic/gin"
	webui "github.com/Yeah114/FunAuth/modules/webui"
	g79client "github.com/Yeah114/g79client"
)

// 统一入口：所有能力都在这里集中分发。
// 这样部署只需要 1~2 个二进制，不需要散放 10 个。
// 子命令：
//   funauth serve                   # Phoenix 验证服务器 + WebUI 控制台 + DB（同进程）
//   funauth serve --no-db --no-webui   # 只跑 Phoenix（需要 DB 时才初始化）
//   funauth webui                   # 只跑 WebUI 控制台（不启动 Phoenix/DB）
//   funauth register                # 批量注册 4399 账号（等同于原 com4399register）
//   funauth register oneshot        # 注册 1 个 4399 账号，输出 username/password
//   funauth sauth <user> <pass>     # 用 4399 账号密码登录拿 SAuth Cookie
//   funauth nickname <user> <pass> [newname]  # 改名为 nklm_XXXXX 或自定义
//   funauth version
//   funauth test bruteforce|com4399|com4399login|proxy|runtest|mcp|skin  [...]
//
// 兼容：不带参数时默认进入 funauth serve（保持老用户无感，和之前 main 一样）

const banner = `
 ___              _   _         _   _
|  _|_ _ ___   __| |_| |_  __ _| |_| |_
|  _| | | . |_|. |  _|   |/ _. |  _|   |
|_| |___/  _|_|___|__|_|_|\__,_|__|_|_|
        |_|  FunAuth Unified CLI
`

var binFlavor = "funauth" // 在 webui 二进制里编译时设置为 webui，用来改变默认子命令

// RunAsFlavor 供外部包（如 cmd/webui）直接调用统一入口，避免重复编译 main。
// flavor: "funauth" 或 "webui"，影响无参数时默认进入的子命令。
// args: 不包含二进制名的参数列表（即 os.Args[1:]）。
func RunAsFlavor(flavor string, args []string) {
	binFlavor = flavor
	runCLI(args)
}

func main() {
	runCLI(os.Args[1:])
}

func runCLI(args []string) {
	log.SetOutput(os.Stdout)
	gin.DefaultWriter = io.MultiWriter(os.Stdout)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stdout)

	if len(args) == 0 {
		args = defaultArgsFor(binFlavor)
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage(binFlavor)
		return
	case "-v", "--version", "version":
		fmt.Printf("FunAuth %s (flavor=%s, go=%s)\n", versionStr(), binFlavor, runtime.Version())
		return
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "serve":
		osExit(runServe(subArgs))
	case "webui":
		osExit(runWebUIOnly(subArgs))
	case "register":
		osExit(runRegister(subArgs))
	case "sauth":
		osExit(runSAuth(subArgs))
	case "nickname":
		osExit(runNickname(subArgs))
	case "test":
		osExit(runTest(subArgs))
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", sub)
		printUsage(binFlavor)
		os.Exit(2)
	}
}

func defaultArgsFor(flavor string) []string {
	switch flavor {
	case "webui":
		return []string{"webui"}
	default:
		return []string{"serve"}
	}
}

func versionStr() string {
	if g79client.EngineVersion != "" {
		return fmt.Sprintf("g79-%s patch-%s", g79client.EngineVersion, g79client.DefaultPatchMetadata().Version)
	}
	return "dev"
}

func osExit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// ================================================================
// serve
// ================================================================

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	noDB := fs.Bool("no-db", false, "跳过 DB 初始化（关闭辅助用户/验证登录功能，仅保留 WebUI+Phoenix 基本端点）")
	noUI := fs.Bool("no-webui", false, "不挂载 WebUI 前端与 /api/webui 端点（只跑 Phoenix/NeoOmega 验证服务器）")
	noPhoenix := fs.Bool("no-phoenix", false, "只跑 WebUI，不挂 Phoenix 验证端点")
	addr := fs.String("addr", "", "监听地址 (默认 FUNAUTH_ADDR 或 :8090)")
	withWebUI := fs.Bool("with-webui", true, "是否挂载 WebUI 控制台（和 --no-webui 相反，二者都保留兼容）")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// 配置交互引导 + DB（除非显式禁用）
	if !*noDB {
		if _, err := db.EnsureConfigInteractive(); err != nil {
			return fmt.Errorf("初始化配置失败: %w", err)
		}
		if err := db.InitDBWithOptions(db.InitOptions{}); err != nil {
			return fmt.Errorf("初始化数据库失败: %w", err)
		}
		// 初始化 com4399 代理池（如果 DB 启动成功）
		initProxyPoolThenCom4399()
	}

	r := buildGinRouter(*noPhoenix, *withWebUI && !*noUI)

	listen := *addr
	if listen == "" {
		listen = os.Getenv("FUNAUTH_ADDR")
	}
	if listen == "" {
		listen = ":8090"
	}
	portkit.FreePortLinuxFromAddr(listen)

	fmt.Print(banner)
	log.Printf("[serve] listening on %s (db=%v phoenix=%v webui=%v)",
		listen, !*noDB, !*noPhoenix, *withWebUI && !*noUI)
	log.Printf("[serve] WebUI: http://localhost%s/ui/", portkit.PrettyAddr(listen))
	return r.Run(listen)
}

func buildGinRouter(withPhoenix, withWebUI bool) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})
	api := r.Group("/api")
	switch {
	case withWebUI && withPhoenix:
		// WebUI 自带 RegisterFBAuthRoutes（它包含与 handlers 等价的 /api/new + /api/phoenix/*，
		// 还额外支持 FBToken）。为避免路由冲突，这里只挂 WebUI，不重复挂 handlers。
		webui.RegisterRoutes(api, r)
	case withPhoenix:
		// 只跑 Phoenix/验证服务器：用 handlers（固定 Cookie 模式）。
		handlers.RegisterNewRoutes(api)
		handlers.RegisterPhoenixRoutes(api)
	case withWebUI:
		// 只跑 WebUI：完整 WebUI + FBToken（含 /api/new /api/phoenix/* 端点）。
		webui.RegisterRoutes(api, r)
	}
	return r
}

// ================================================================
// webui (独立版，默认 webui 二进制的 main 就走这里)
// ================================================================

func runWebUIOnly(args []string) error {
	fs := flag.NewFlagSet("webui", flag.ContinueOnError)
	addr := fs.String("addr", "", "监听地址 (默认 FUNAUTH_ADDR 或 :8090)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})
	api := r.Group("/api")
	webui.RegisterRoutes(api, r)

	listen := *addr
	if listen == "" {
		listen = os.Getenv("FUNAUTH_ADDR")
	}
	if listen == "" {
		listen = ":8090"
	}
	portkit.FreePortLinuxFromAddr(listen)
	log.Printf("[webui] listening on %s", listen)
	log.Printf("[webui] 控制台地址: http://localhost%s/ui/", portkit.PrettyAddr(listen))
	return r.Run(listen)
}

// ================================================================
// register / sauth / nickname
// ================================================================

func runRegister(args []string) error {
	if len(args) == 0 {
		// 默认：跑批（等同于原 com4399register-linux-amd64）
		return com4399cli.RunRegisterCookieLegacy()
	}
	switch args[0] {
	case "oneshot":
		u, p, err := com4399cli.RunOneShot(context.Background(), "", "", "", "")
		if err != nil {
			return err
		}
		fmt.Printf("username=%s\npassword=%s\n", u, p)
		return nil
	case "sauth":
		u, p, c, err := com4399cli.RegisterAndGetSAuth(context.Background(), "", "", "", "")
		if err != nil {
			return err
		}
		fmt.Printf("username=%s\npassword=%s\n=====SAUTH=====\n%s\n", u, p, c)
		return nil
	default:
		return portkit.Usagef("register: 子命令可选 <oneshot|sauth>；不带参数则跑批注册")
	}
}

func runSAuth(args []string) error {
	if len(args) < 2 {
		return portkit.Usagef("sauth: 需要 <username> <password>")
	}
	c, err := com4399cli.RunLoginOnce(context.Background(), args[0], args[1])
	if err != nil {
		return err
	}
	fmt.Println(c)
	return nil
}

func runNickname(args []string) error {
	if len(args) < 2 {
		return portkit.Usagef("nickname: 需要 <username> <password> [newname]  —— 若留空 newname 则自动用 nklm_XXXXX")
	}
	user, pass := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
	if user == "" || pass == "" {
		return portkit.Usagef("nickname: username/password 不能为空")
	}
	ctx := context.Background()
	cookie, err := com4399cli.RunLoginOnce(ctx, user, pass)
	if err != nil {
		return fmt.Errorf("登录拿 SAuth: %w", err)
	}
	cli, err := g79client.NewClient()
	if err != nil {
		return err
	}
	if err := cli.G79AuthenticateWithCookie(cookie); err != nil {
		return fmt.Errorf("G79 认证: %w", err)
	}
	target := ""
	if len(args) >= 3 {
		target = strings.TrimSpace(args[2])
	}
	// 默认走统一逻辑：空名字自动 nklm_XXXXX；有名字也强制改名时仍优先 nklm_（不强制覆盖已有合法名，这里允许显式 newname 覆盖）
	if target == "" {
		if err := g79client.EnsureNicknameNKLMIfEmpty(cli, "NKLM"); err != nil {
			return err
		}
		fmt.Printf("确保昵称完成: name=%s\n", cli.UserDetail.Name)
		return nil
	}
	if err := cli.UpdateNickname(target); err != nil {
		return err
	}
	fmt.Printf("昵称已改为: %s\n", target)
	return nil
}

// ================================================================
// test <name> —— 通过调用编译好的独立二进制（与 file/new 一致）实现。
// 这样不破坏原 test-* 入口代码，只负责转发。
// ================================================================

var knownTests = map[string]string{
	"bruteforce":   "test-bruteforce-linux-amd64",
	"com4399":      "test-com4399-linux-amd64",
	"com4399login": "test-com4399login-linux-amd64",
	"proxy":        "test-proxy-linux-amd64",
	"runtest":      "test-runtest-linux-amd64",
	"mcp":          "test-test_mcp-linux-amd64",
	"skin":         "test-test_skin-linux-amd64",
}

func runTest(args []string) error {
	if len(args) == 0 {
		names := make([]string, 0, len(knownTests))
		for k := range knownTests {
			names = append(names, k)
		}
		return portkit.Usagef("test: 需要子命令。可用: %s", strings.Join(names, " / "))
	}
	name := args[0]
	rest := args[1:]
	binName, ok := knownTests[name]
	if !ok {
		return portkit.Usagef("test: 未知测试名 %q", name)
	}
	bin, err := resolveTestBinary(binName)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, rest...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() >= 0 {
			os.Exit(ee.ExitCode())
		}
		return fmt.Errorf("run test %s: %w", name, err)
	}
	return nil
}

// resolveTestBinary 按优先级找 test 二进制：
// 1) 与当前可执行文件同目录（例如 file/unified/ 下把所有 test 工具也放进来即可）
// 2) ../file/new/ （构建脚本的产物目录）
// 3) PATH
func resolveTestBinary(binName string) (string, error) {
	candidates := []string{binName}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, binName))
		candidates = append(candidates, filepath.Join(dir, "..", "file", "new", binName))
	}
	candidates = append(candidates, "/workspace/file/new/"+binName)
	candidates = append(candidates, "/workspace/file/"+binName)
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			// 可执行权限粗略校验
			if st.Mode().Perm()&0o111 != 0 || runtime.GOOS == "windows" {
				return p, nil
			}
		}
	}
	if full, err := exec.LookPath(binName); err == nil {
		return full, nil
	}
	return "", fmt.Errorf("找不到测试二进制 %s。请先执行构建把所有二进制放到 file/unified/ 或 file/new/ 目录", binName)
}

// ================================================================
// usage
// ================================================================

func printUsage(flavor string) {
	bin := "funauth-linux-amd64"
	if flavor == "webui" {
		bin = "webui-linux-amd64"
	}
	fmt.Printf("%s\n", strings.TrimSpace(banner))
	fmt.Printf("用法:\n  %s [子命令] [选项...]\n\n", bin)
	fmt.Println("子命令:")
	fmt.Println("  serve                     默认：启动一体化 HTTP 服务")
	fmt.Println("     --no-db               跳过 DB + 辅助用户（只保留 API 层）")
	fmt.Println("     --no-webui            不挂 WebUI 控制台（只当 Phoenix/NeoOmega 验证服务器）")
	fmt.Println("     --no-phoenix          不挂 Phoenix/验证端点（只当 Bunker-Web）")
	fmt.Println("     --addr :8090          监听地址（或用 FUNAUTH_ADDR 环境变量）")
	fmt.Println("  webui                     纯 WebUI 控制台（独立二进制 webui-linux-amd64 默认进这个）")
	fmt.Println("  register                  批量注册 4399（原来的 com4399register-linux-amd64）")
	fmt.Println("  register oneshot          注册 1 个 4399 账号，输出 username/password")
	fmt.Println("  register sauth            注册 1 个 + 登录并拿完整 SAuth Cookie")
	fmt.Println("  sauth <user> <pass>       用 4399 账号密码登录，打印 SAuth Cookie")
	fmt.Println("  nickname <u> <p> [name]   辅助账号空名时自动改成 nklm_XXXXX；也可显式指定名字")
	fmt.Println("  test bruteforce           原 test-bruteforce-linux-amd64")
	fmt.Println("  test com4399              原 test-com4399-linux-amd64")
	fmt.Println("  test com4399login         原 test-com4399login-linux-amd64")
	fmt.Println("  test proxy                原 test-proxy-linux-amd64")
	fmt.Println("  test runtest              原 test-runtest-linux-amd64")
	fmt.Println("  test mcp                  原 test-test_mcp-linux-amd64")
	fmt.Println("  test skin                 原 test-test_skin-linux-amd64")
	fmt.Println("  version                   打印版本")
	fmt.Println("  help / -h / --help        查看帮助")
}
