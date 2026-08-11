package com4399

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateWebRegisterRequest(t *testing.T) {
	req := WebRegisterRequest{
		Username: "User12345",
		Password: "Pass12345",
		RealName: "张三",
		IDCard:   "11010519491231002X",
	}
	if err := validateWebRegisterRequest(req); err != nil {
		t.Fatalf("validateWebRegisterRequest failed: %v", err)
	}
}

func TestEncodeWebFormKeepsDuplicatePassword(t *testing.T) {
	encoded := encodeWebForm([]webFormField{
		{Name: "password", Value: "encrypted"},
		{Name: "username", Value: "user"},
		{Name: "password", Value: ""},
	})
	if strings.Count(encoded, "password=") != 2 {
		t.Fatalf("expected duplicate password fields, got %s", encoded)
	}
}

func TestExtractWebCaptchaChallenge(t *testing.T) {
	doc := `<input name="captcha_id" value="abc123"><input name="reg_req_id" value="req1"><img id="captcha_img" src="/ptlogin/captcha.do?captchaId=abc123">`
	challenge := extractWebCaptchaChallenge(doc, loginBaseURL)
	if challenge == nil || challenge.CaptchaID != "abc123" || challenge.RegRequestID != "req1" {
		t.Fatalf("unexpected challenge: %+v", challenge)
	}
	if !strings.HasPrefix(challenge.CaptchaURL, loginBaseURL) {
		t.Fatalf("unexpected captcha url: %s", challenge.CaptchaURL)
	}
}

func TestEncryptWebRegisterAESOpenSSLPayload(t *testing.T) {
	ciphertext, err := encryptWebRegisterAES("Pass12345")
	if err != nil {
		t.Fatalf("encryptWebRegisterAES failed: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		t.Fatalf("ciphertext is not base64: %v", err)
	}
	if len(raw) < 16 || string(raw[:8]) != "Salted__" {
		t.Fatalf("unexpected openssl payload prefix")
	}
}
