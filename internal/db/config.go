package db

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Config 描述 funauth 的可持久化配置，存放在 config.json 中
type Config struct {
	MySQL MySQLConfig `json:"mysql"`
}

// MySQLConfig 描述数据库连接参数；提供 DSN 直填 或 host/port/user/password/database 组合两种方式
type MySQLConfig struct {
	// DSN 优先：完整 DSN 字符串，例如 user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	DSN string `json:"dsn,omitempty"`

	// 当 DSN 为空时，使用以下字段拼接
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	// Params 默认 "charset=utf8mb4&parseTime=True&loc=Local"
	Params string `json:"params,omitempty"`
}

const (
	defaultDBParams = "charset=utf8mb4&parseTime=True&loc=Local"
	defaultDBHost   = "127.0.0.1"
	defaultDBPort   = 3306
	defaultDBUser   = "authservice"
	defaultDBName   = "authservice"
)

// global overrides（供进程启动期间注入使用，或通过环境变量设置）
var (
	dsnOverride  string
	cachedConfig *Config
)

// SetDSNOverride 显式覆盖 DSN（优先级最高）
func SetDSNOverride(dsn string) {
	dsnOverride = strings.TrimSpace(dsn)
}

// DefaultConfigPath 返回 config.json 的查找顺序结果：
// 1. FUNAUTH_CONFIG 环境变量
// 2. ./config.json（当前工作目录）
// 3. <可执行文件目录>/config.json
func DefaultConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("FUNAUTH_CONFIG")); p != "" {
		return p
	}
	cwd, _ := os.Getwd()
	p1 := filepath.Join(cwd, "config.json")
	if _, err := os.Stat(p1); err == nil {
		return p1
	}
	if exec, err := os.Executable(); err == nil {
		p2 := filepath.Join(filepath.Dir(exec), "config.json")
		if _, err := os.Stat(p2); err == nil {
			return p2
		}
	}
	return p1 // 默认写回 cwd
}

// LoadConfig 读取并解析 config.json；文件不存在时返回 (nil, nil)，不报错
func LoadConfig() (*Config, error) {
	path := DefaultConfigPath()
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("解析 config.json 失败: %w", err)
	}
	cachedConfig = &cfg
	return cachedConfig, nil
}

// SaveConfig 将配置以漂亮的 JSON 格式写入 DefaultConfigPath
func SaveConfig(cfg *Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config 不能为空")
	}
	path := DefaultConfigPath()
	// 确保目录存在
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("创建目录失败: %w", err)
		}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", fmt.Errorf("写入 config.json 失败: %w", err)
	}
	return path, nil
}

// ResolveDSN 按照优先级拼装最终的 MySQL DSN：
// 1. SetDSNOverride()
// 2. FUNAUTH_MYSQL_DSN 环境变量
// 3. config.MySQL.DSN
// 4. config.MySQL.{host,port,username,password,database,params}
// 5. 默认占位符 (历史值)
func ResolveDSN(cfg *Config) string {
	// 1. override
	if strings.TrimSpace(dsnOverride) != "" {
		return dsnOverride
	}
	// 2. env
	if env := strings.TrimSpace(os.Getenv("FUNAUTH_MYSQL_DSN")); env != "" {
		return env
	}
	if cfg == nil {
		cfg = cachedConfig
	}
	if cfg != nil {
		m := cfg.MySQL
		// 3. 完整 DSN
		if strings.TrimSpace(m.DSN) != "" {
			return m.DSN
		}
		// 4. 拼接
		host := firstNonEmpty(m.Host, defaultDBHost)
		port := m.Port
		if port <= 0 {
			port = defaultDBPort
		}
		user := firstNonEmpty(m.Username, defaultDBUser)
		pass := m.Password
		dbname := firstNonEmpty(m.Database, defaultDBName)
		params := firstNonEmpty(m.Params, defaultDBParams)
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", user, pass, host, port, dbname, params)
	}
	// 5. 兼容默认占位
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", defaultDBUser, "密码", defaultDBHost, defaultDBPort, defaultDBName, defaultDBParams)
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}

// EnsureConfigInteractive 在配置缺失/DSN 仍然占位（含明文"密码"）或数据库连接失败时，
// 在 TTY 上交互式询问用户 MySQL 参数并保存到 config.json。
// 非 TTY 环境：若 DSN 不可用则直接返回错误提示，避免卡住。
func EnsureConfigInteractive() (*Config, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &Config{}
	}

	// 环境变量已明确提供 DSN：不弹交互
	if strings.TrimSpace(dsnOverride) != "" || strings.TrimSpace(os.Getenv("FUNAUTH_MYSQL_DSN")) != "" {
		return cfg, nil
	}

	// 如果 DSN 仍然使用占位"密码"或必填项为空 → 需要引导
	dsn := ResolveDSN(cfg)
	needWizard := strings.Contains(dsn, ":密码@") ||
		strings.TrimSpace(cfg.MySQL.DSN) == "" && (cfg.MySQL.Host == "" || cfg.MySQL.Username == "")

	if !needWizard {
		return cfg, nil
	}

	// TTY 判定
	stdinFd := int(os.Stdin.Fd())
	isTTY := isTerminal(stdinFd)
	if !isTTY {
		fmt.Println()
		fmt.Println("=============================================")
		fmt.Println("[config] 首次运行需要 MySQL 配置，但当前环境非交互（stdin 不是终端）。")
		fmt.Println("可选方案：")
		fmt.Println("  1) 设置环境变量 FUNAUTH_MYSQL_DSN='user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local'")
		fmt.Println("  2) 配置 config.json：")
		fmt.Println(`       {"mysql":{"host":"127.0.0.1","port":3306,"username":"authservice","password":"xxx","database":"authservice"}}`)
		fmt.Println("       或:")
		fmt.Println(`       {"mysql":{"dsn":"authservice:xxx@tcp(127.0.0.1:3306)/authservice?charset=utf8mb4&parseTime=True&loc=Local"}}`)
		fmt.Println("  3) 手动启动终端交互模式： FUNAUTH_FORCE_WIZARD=1 <二进制>")
		fmt.Println("=============================================")
		fmt.Println()
		return cfg, nil
	}

	fmt.Println()
	fmt.Println("=============================================")
	fmt.Println("🎯 欢迎使用 FunAuth！")
	fmt.Println("   首次运行，请输入 MySQL 连接信息：")
	fmt.Println("=============================================")
	reader := bufio.NewReader(os.Stdin)
	ask := func(prompt, def string) string {
		if def != "" {
			fmt.Printf("%s (默认：%s) > ", prompt, def)
		} else {
			fmt.Printf("%s > ", prompt)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return def
		}
		line = strings.TrimRight(line, "\r\n")
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		return line
	}
	cur := cfg.MySQL
	host := ask("1. MySQL Host", firstNonEmpty(cur.Host, defaultDBHost))
	portStr := ask("2. MySQL Port", i2s(firstPositive(cur.Port, defaultDBPort)))
	user := ask("3. MySQL 用户名", firstNonEmpty(cur.Username, defaultDBUser))
	pass := ask("4. MySQL 密码", cur.Password)
	dbname := ask("5. 数据库名", firstNonEmpty(cur.Database, defaultDBName))
	port, _ := strconv.Atoi(portStr)
	if port <= 0 {
		port = defaultDBPort
	}
	cfg.MySQL = MySQLConfig{
		Host:     host,
		Port:     port,
		Username: user,
		Password: pass,
		Database: dbname,
		Params:   firstNonEmpty(cur.Params, defaultDBParams),
	}
	out, err := SaveConfig(cfg)
	if err != nil {
		fmt.Printf("[config] 保存配置失败: %v\n", err)
		return cfg, err
	}
	fmt.Printf("\n[config] 配置已保存到：%s\n", out)
	fmt.Println("=============================================")
	fmt.Println()
	return cfg, nil
}

func i2s(i int) string { return strconv.Itoa(i) }
func firstPositive(x, fallback int) int {
	if x > 0 {
		return x
	}
	return fallback
}

// IsConfigReady 判断数据库配置是否就绪（可用于启动时决定是否进入 web setup 向导）
// 优先级：FUNAUTH_MYSQL_DSN 环境变量 > config.json
// 都没有，或仍是占位"密码"，则视为未就绪
func IsConfigReady() bool {
	if strings.TrimSpace(os.Getenv("FUNAUTH_MYSQL_DSN")) != "" {
		return true
	}
	if strings.TrimSpace(dsnOverride) != "" {
		return true
	}
	cfg, err := LoadConfig()
	if err != nil || cfg == nil {
		return false
	}
	dsn := ResolveDSN(cfg)
	if strings.Contains(dsn, ":密码@") {
		return false
	}
	if cfg.MySQL.Host == "" && cfg.MySQL.Username == "" && strings.TrimSpace(cfg.MySQL.DSN) == "" {
		return false
	}
	return true
}

// -------- minimal isatty --------
// 避免引入 golang.org/x/term 依赖
func isTerminal(fd int) bool {
	if os.Getenv("FUNAUTH_FORCE_WIZARD") != "" {
		return true
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return false
	}
	// 1. 必须是字符设备
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return false
	}
	// 2. 不能是 /dev/null, /dev/zero 这种空设备（也是字符设备但不可交互）
	//    Linux 下通过 /proc/self/fd/0 的软链目标判断
	if runtime.GOOS == "linux" {
		if target, err := os.Readlink("/proc/self/fd/0"); err == nil {
			base := target
			if idx := lastSlash(target); idx >= 0 {
				base = target[idx+1:]
			}
			switch base {
			case "null", "zero", "urandom", "random", "full":
				return false
			}
			// /dev/stdin 指向 pipe 时也是非交互
			if strings.HasPrefix(target, "pipe:") {
				return false
			}
		}
	}
	return true
}

func lastSlash(s string) int {
	return strings.LastIndex(s, "/")
}
