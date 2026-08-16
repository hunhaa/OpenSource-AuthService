package portkit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ParsePort 把 addr 形式的 ":8080" / "127.0.0.1:8080" / "8080" 解析成端口号
func ParsePort(addr string) (int, bool) {
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

// PrettyAddr 日志里展示用，"0.0.0.0:8080" 直接返回，":8080" 保持不变
func PrettyAddr(a string) string {
	if strings.HasPrefix(a, ":") {
		return a
	}
	return a
}

// FreePortLinux 如果在 Linux 上运行，尝试释放占用 port 的进程。非 Linux 直接 no-op。
func FreePortLinux(port int) {
	if runtime.GOOS != "linux" {
		return
	}
	if port <= 0 || port > 65535 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pids := findPidsWithSS(ctx, port)
	if len(pids) == 0 {
		pids = findPidsWithLsof(ctx, port)
	}
	if len(pids) == 0 {
		log.Printf("[port] no owner found for port %d", port)
		return
	}
	uniq := make(map[int]struct{})
	for _, p := range pids {
		uniq[p] = struct{}{}
	}
	for pid := range uniq {
		if pid <= 1 || pid == os.Getpid() {
			continue
		}
		log.Printf("[port] SIGTERM pid=%d (port %d)", pid, port)
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	time.Sleep(1200 * time.Millisecond)
	for pid := range uniq {
		if pid <= 1 || pid == os.Getpid() {
			continue
		}
		if Alive(pid) {
			log.Printf("[port] SIGKILL pid=%d (port %d)", pid, port)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

// FreePortLinuxFromAddr 接收 addr 字符串形式的便捷封装
func FreePortLinuxFromAddr(addr string) {
	if p, ok := ParsePort(addr); ok {
		log.Printf("[port] try free port %d before binding", p)
		FreePortLinux(p)
	}
}

func Alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func findPidsWithSS(ctx context.Context, port int) []int {
	ss, err := exec.LookPath("ss")
	if err != nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, ss, "-lntp").CombinedOutput()
	if err != nil || len(out) == 0 {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	var pids []int
	needle := ":" + strconv.Itoa(port) + " "
	for _, ln := range lines {
		if !strings.Contains(ln, needle) {
			continue
		}
		s := ln
		for {
			idx := strings.Index(s, "pid=")
			if idx < 0 {
				break
			}
			s = s[idx+4:]
			j := 0
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if j > 0 {
				if v, err := strconv.Atoi(s[:j]); err == nil {
					pids = append(pids, v)
				}
				s = s[j:]
			} else {
				break
			}
		}
	}
	return pids
}

func findPidsWithLsof(ctx context.Context, port int) []int {
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return nil
	}
	argsSets := [][]string{
		{"-t", "-iTCP:" + strconv.Itoa(port), "-sTCP:LISTEN"},
		{"-t", "-i:" + strconv.Itoa(port)},
	}
	for _, args := range argsSets {
		cmd := exec.CommandContext(ctx, lsof, args...)
		out, err := cmd.CombinedOutput()
		if err != nil || len(out) == 0 {
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		var pids []int
		for _, ln := range lines {
			if v, err := strconv.Atoi(strings.TrimSpace(ln)); err == nil {
				pids = append(pids, v)
			}
		}
		if len(pids) > 0 {
			return pids
		}
	}
	return nil
}

// ErrUsage 是子命令自定义的 usage 错误（一般 -h / 缺少参数时触发）
var ErrUsage = errors.New("usage")

func Usagef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUsage, fmt.Sprintf(format, args...))
}
