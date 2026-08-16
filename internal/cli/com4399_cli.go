package com4399cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	com4399tools "github.com/Yeah114/FunAuth/utils/com4399"
	account4399 "github.com/Yeah114/g79client/account/com4399"
)

// RunOneShot 注册 1 个 4399 账号（只注册 + 实名），返回 username/password
func RunOneShot(ctx context.Context, username, password, realName, idCard string) (string, string, error) {
	var gotU, gotP string
	builder := com4399tools.NewCom4399CookieRegister().
		WithContext(ctx).
		WithTimeout(60 * time.Second).
		WithAutoCaptcha(true).
		WithSkipLogin(true).
		WithGeneratedAccountCallback(func(u, p string) {
			gotU, gotP = u, p
		})
	if username == "" || password == "" {
		builder = builder.WithAutoProxy(true)
	} else {
		builder = builder.WithAccount(username, password).WithAutoProxy(false)
		gotU, gotP = username, password
	}
	if realName != "" && idCard != "" {
		builder = builder.WithRealName(realName, idCard)
	}
	_, err := builder.RegisterCookie()
	if err != nil {
		return "", "", fmt.Errorf("register oneshot: %w", err)
	}
	if gotU == "" || gotP == "" {
		return "", "", fmt.Errorf("register oneshot: 未能获取账号密码（回调未触发）")
	}
	return gotU, gotP, nil
}

// RunLoginOnce 用 4399 账号密码登录拿 SAuth cookie 字符串（可直接喂给 G79AuthenticateWithCookie）
func RunLoginOnce(ctx context.Context, username, password string) (string, error) {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("username and password are required")
	}
	cookie, err := account4399.LoginCookieWithPassword(ctx, strings.TrimSpace(username), strings.TrimSpace(password))
	if err != nil {
		return "", fmt.Errorf("login4399: %w", err)
	}
	return cookie, nil
}

// RunRegisterCookieLegacy 批量跑批：通过调用独立的 com4399register-linux-amd64 二进制实现。
// 不直接 import cmd/com4399register（它是 main 包）。
func RunRegisterCookieLegacy() error {
	binName := "com4399register-linux-amd64"
	if runtime.GOOS == "windows" {
		binName = "com4399register-windows-amd64.exe"
	}
	bin, err := resolveCom4399RegisterBinary(binName)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() >= 0 {
			os.Exit(ee.ExitCode())
		}
		return fmt.Errorf("run com4399register: %w", err)
	}
	return nil
}

func resolveCom4399RegisterBinary(binName string) (string, error) {
	candidates := []string{binName}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, binName))
		candidates = append(candidates, filepath.Join(dir, "..", "file", "new", binName))
		candidates = append(candidates, filepath.Join(dir, "..", "file", binName))
	}
	candidates = append(candidates, "/workspace/file/new/"+binName)
	candidates = append(candidates, "/workspace/file/"+binName)
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if st.Mode().Perm()&0o111 != 0 || runtime.GOOS == "windows" {
				return p, nil
			}
		}
	}
	if full, err := exec.LookPath(binName); err == nil {
		return full, nil
	}
	return "", fmt.Errorf("找不到 com4399register 二进制 %s。请先构建后放到 file/unified/ 或 file/new/ 目录，或放到 PATH 中", binName)
}

// RegisterAndGetSAuth 一步到位：注册 + 登录 + 返回 SAuth cookie；如果传入 username/password 就复用那对账号。
func RegisterAndGetSAuth(ctx context.Context, username, password, realName, idCard string) (u, p, cookie string, err error) {
	u, p, err = RunOneShot(ctx, username, password, realName, idCard)
	if err != nil {
		return
	}
	cookie, err = RunLoginOnce(ctx, u, p)
	if err != nil {
		return
	}
	return
}

// EnsureLog 统一设置 log 到 stdout（避免部分面板不抓 stderr）
func EnsureLog() {
	log.SetOutput(os.Stdout)
}
