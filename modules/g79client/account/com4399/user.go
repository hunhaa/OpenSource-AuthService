package com4399

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	defaultAimInfo = `{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":"Asia/Shanghai","celluar_ip":"","operator":"","is_vpn_enabled":false}`
	defaultRAM     = "1035337728"
	defaultROM     = "134208294912"
)

// Sauth 描述 g79client Cookie 中的 sauth_json。
type Sauth struct {
	AimInfo          string `json:"aim_info"`
	AppChannel       string `json:"app_channel"`
	ClientLoginSN    string `json:"client_login_sn"`
	DeviceID         string `json:"deviceid"`
	GameID           string `json:"gameid"`
	GasToken         string `json:"gas_token"`
	GetAccessToken   string `json:"get_access_token,omitempty"`
	IP               string `json:"ip"`
	IsUnisdkGuest    int    `json:"is_unisdk_guest"`
	LoginChannel     string `json:"login_channel"`
	Platform         string `json:"platform"`
	RealName         string `json:"realname,omitempty"`
	SDKVersion       string `json:"sdk_version"`
	SDKUID           string `json:"sdkuid"`
	SessionID        string `json:"sessionid"`
	SourceAppChannel string `json:"source_app_channel,omitempty"`
	SourcePlatform   string `json:"source_platform"`
	Timestamp        string `json:"timestamp,omitempty"`
	UDID             string `json:"udid"`
	UserID           string `json:"userid,omitempty"`
}

// User 描述 4399 登录后的 Cookie 生成数据。
type User struct {
	Sauth    Sauth    `json:"sauth"`
	Info     UserInfo `json:"info"`
	Mac      string   `json:"mac"`
	Emulator int      `json:"emulator"`
	IsGuest  bool     `json:"is_guest"`
	Ram      string   `json:"ram"`
	Rom      string   `json:"rom"`
}

func newUser(info *UserInfo) *User {
	clientLoginSN := strings.ReplaceAll(uuid.NewString(), "-", "")
	udid, err := randomHexString(8)
	if err != nil {
		udid = clientLoginSN[:16]
	}
	mac, err := randomHexString(16)
	if err != nil {
		mac = clientLoginSN
	}

	return &User{
		Sauth: Sauth{
			AimInfo:          defaultAimInfo,
			AppChannel:       "4399com",
			ClientLoginSN:    clientLoginSN,
			DeviceID:         clientLoginSN,
			GameID:           "x19",
			GasToken:         "",
			GetAccessToken:   "1",
			IP:               "127.0.0.1",
			IsUnisdkGuest:    0,
			LoginChannel:     "4399com",
			Platform:         "ad",
			RealName:         `{"realname_type":"0"}`,
			SDKVersion:       "1.0.0",
			SDKUID:           strconv.FormatInt(info.UID, 10),
			SessionID:        info.State,
			SourceAppChannel: "4399com",
			SourcePlatform:   "ad",
			Timestamp:        "",
			UDID:             udid,
			UserID:           "",
		},
		Info:     *info,
		Mac:      mac,
		Emulator: 1,
		IsGuest:  false,
		Ram:      defaultRAM,
		Rom:      defaultROM,
	}
}

// CookieString 返回 g79client.G79AuthenticateWithCookie 可直接使用的 Cookie 字符串。
func (u *User) CookieString() (string, error) {
	if u == nil {
		return "", errors.New("com4399: user 不能为空")
	}
	sauthJSON, err := json.Marshal(u.Sauth)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"sauth_json": string(sauthJSON),
		"mac_addr":   u.Mac,
		"ram":        u.Ram,
		"rom":        u.Rom,
		"is_guest":   u.IsGuest,
		"emulator":   u.Emulator,
	}
	cookie, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(cookie), nil
}
