package com4399

import (
	"encoding/json"
	"testing"
)

func TestCookieStringIncludesCom4399Sauth(t *testing.T) {
	user := newUser(&UserInfo{
		UID:   123456,
		State: "state-token",
	})

	cookie, err := user.CookieString()
	if err != nil {
		t.Fatalf("CookieString failed: %v", err)
	}

	var payload struct {
		SauthJSON string `json:"sauth_json"`
		MacAddr   string `json:"mac_addr"`
		RAM       string `json:"ram"`
		ROM       string `json:"rom"`
		IsGuest   bool   `json:"is_guest"`
		Emulator  int    `json:"emulator"`
	}
	if err := json.Unmarshal([]byte(cookie), &payload); err != nil {
		t.Fatalf("parse cookie failed: %v", err)
	}
	var sauth Sauth
	if err := json.Unmarshal([]byte(payload.SauthJSON), &sauth); err != nil {
		t.Fatalf("parse sauth failed: %v", err)
	}

	if sauth.SDKUID != "123456" || sauth.SessionID != "state-token" {
		t.Fatalf("unexpected sauth account fields: %+v", sauth)
	}
	if sauth.LoginChannel != "4399com" || sauth.AppChannel != "4399com" || sauth.Platform != "ad" {
		t.Fatalf("unexpected sauth channel fields: %+v", sauth)
	}
	if payload.MacAddr == "" || payload.RAM == "" || payload.ROM == "" || payload.IsGuest || payload.Emulator != 1 {
		t.Fatalf("unexpected cookie payload: %+v", payload)
	}
}
