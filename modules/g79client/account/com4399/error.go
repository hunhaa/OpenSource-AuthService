package com4399

import "fmt"

// AccountNotFoundError 表示 4399 返回账号不存在或尚未注册。
type AccountNotFoundError struct {
	Username string
	Message  string
}

func (e *AccountNotFoundError) Error() string {
	if e == nil {
		return "com4399: 账号不存在或尚未注册"
	}
	if e.Username != "" {
		return fmt.Sprintf("com4399: 账号 %s 不存在或尚未注册", e.Username)
	}
	if e.Message != "" {
		return fmt.Sprintf("com4399: %s", e.Message)
	}
	return "com4399: 账号不存在或尚未注册"
}

// NeedCaptchaError 表示 4399 登录流程要求输入验证码。
type NeedCaptchaError struct {
	Reason     string
	CaptchaID  string
	CaptchaURL string
}

func (e *NeedCaptchaError) Error() string {
	if e == nil {
		return "com4399: 需要验证码"
	}
	if e.CaptchaURL != "" {
		return fmt.Sprintf("com4399: %s captcha_id=%s captcha_url=%s", e.Reason, e.CaptchaID, e.CaptchaURL)
	}
	return fmt.Sprintf("com4399: %s", e.Reason)
}
