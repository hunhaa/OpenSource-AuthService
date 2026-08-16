package main

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func lookupCmd(ctx context.Context, name string) (string, error) {
	return exec.LookPath(name)
}

func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// 从 `ss -ltnp` 或 `netstat -ltnp` 输出解析占用 port 的进程 pid
func parsePidFromNetPortOutput(out []byte, port int, tool string) []int {
	re := regexp.MustCompile(`pid=(\d+)`)
	reFallback := regexp.MustCompile(`(\d+)/`)
	rePort := regexp.MustCompile(`[:\.]` + strconv.Itoa(port) + `\b`)
	var pids []int
	seen := map[int]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !rePort.MatchString(line) {
			continue
		}
		m := re.FindStringSubmatch(line)
		if m != nil {
			if v, e := strconv.Atoi(m[1]); e == nil {
				if _, ok := seen[v]; !ok { seen[v] = struct{}{}; pids = append(pids, v) }
			}
			continue
		}
		m2 := reFallback.FindStringSubmatch(line)
		if m2 != nil {
			if v, e := strconv.Atoi(m2[1]); e == nil {
				if _, ok := seen[v]; !ok { seen[v] = struct{}{}; pids = append(pids, v) }
			}
		}
	}
	return pids
}

// 从 `lsof -iTCP:PORT -sTCP:LISTEN -t` 输出 (每行一个 pid)
func parsePidFromLsofT(out []byte) []int {
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		if v, err := strconv.Atoi(line); err == nil && v > 0 {
			pids = append(pids, v)
		}
	}
	return pids
}
