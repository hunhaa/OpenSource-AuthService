package com4399

import "testing"

func TestIsAccountNotFoundPage(t *testing.T) {
	html := `<html><body><p class="form-link">没有账号？<a href="#" id="show-register">注册</a></p><label>账号</label><label>密码</label></body></html>`
	message := extractHTMLMessage(html)
	if !isAccountNotFoundPage(html, message) {
		t.Fatalf("expected account not found page, message=%q", message)
	}
}

func TestPasswordErrorIsNotAccountNotFound(t *testing.T) {
	html := `<html><body>用户名或密码错误</body></html>`
	message := extractHTMLMessage(html)
	if isAccountNotFoundPage(html, message) {
		t.Fatalf("password error should not be account not found, message=%q", message)
	}
}
