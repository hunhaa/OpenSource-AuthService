package g79client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AuthenticationUpdateResponse 表示 /authentication/update 的响应。
type AuthenticationUpdateResponse struct {
	Response
	Entity struct {
		EntityID string `json:"entity_id"`
		Token    string `json:"token"`
	} `json:"entity"`
}

// AuthenticationUpdate 调用 /authentication/update 刷新 token，
// 成功后自动更新 Client.UserToken。
func (c *Client) AuthenticationUpdate() (*AuthenticationUpdateResponse, error) {
	api := "/authentication/update"
	payload := map[string]any{"entity_id": c.UserID}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	encrypted, err := G79HttpEncrypt(jsonData)
	if err != nil {
		return nil, fmt.Errorf("加密请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.WebServerUrl+api, strings.NewReader(hex.EncodeToString(encrypted)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, string(jsonData), c.UserToken))

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	encBytes, err := hex.DecodeString(string(respBody))
	if err != nil {
		return nil, fmt.Errorf("hex解码失败: %w", err)
	}

	decrypted, err := G79HttpDecrypt(encBytes)
	if err != nil {
		return nil, fmt.Errorf("解密响应失败: %w", err)
	}

	validJSON := GetValidJSON(decrypted)
	var result AuthenticationUpdateResponse
	if err := json.Unmarshal(validJSON, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("刷新失败 (code=%d): %s", result.Code, result.Message)
	}
	if result.Entity.Token != "" {
		c.UserToken = result.Entity.Token
	}

	return &result, nil
}

func (c *Client) UpdateToken() (string, error) {
	resp, err := c.AuthenticationUpdate()
	if err != nil {
		return "", err
	}
	if resp.Entity.Token != "" {
		return resp.Entity.Token, nil
	}
	return "", nil
}
