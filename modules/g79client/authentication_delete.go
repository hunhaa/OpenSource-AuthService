package g79client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AuthenticationDeleteResponse 表示 /authentication/delete 的响应。
type AuthenticationDeleteResponse struct {
	Response
}

// AuthenticationDelete 调用 /authentication/delete 注销会话。
// 请求体经 G79HttpEncrypt 加密后以十六进制字符串提交，并使用 CalculateDynamicToken 作为 user-token。
// logoutType 通常传 0（与官方客户端默认一致）；成功后会清空 Client.UserToken。
func (c *Client) AuthenticationDelete(logoutType int) (*AuthenticationDeleteResponse, error) {
	api := "/authentication/delete"
	payload := map[string]any{
		"user_id":     c.UserID,
		"logout_type": logoutType,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	encrypted, err := G79HttpEncrypt(jsonData)
	if err != nil {
		return nil, fmt.Errorf("加密注销请求失败: %w", err)
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
	var result AuthenticationDeleteResponse
	if err := json.Unmarshal(validJSON, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("注销失败 (code=%d): %s", result.Code, result.Message)
	}

	c.UserToken = ""
	return &result, nil
}
