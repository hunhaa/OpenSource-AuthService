package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// CCVoiceSetPlayerStateResponse 描述 /cc-audio/set-domain-server-player-voice-state 的响应。
type CCVoiceSetPlayerStateResponse struct {
	Response
	Entity any `json:"entity"`
}

// SetCCVoicePlayerState 设置山头服内指定玩家的语音禁用状态。
func (c *Client) SetCCVoicePlayerState(sid string, uid string, isBan bool) (*CCVoiceSetPlayerStateResponse, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, fmt.Errorf("SetCCVoicePlayerState: sid 不能为空")
	}
	if strings.TrimSpace(uid) == "" {
		return nil, fmt.Errorf("SetCCVoicePlayerState: uid 不能为空")
	}

	api := "/cc-audio/set-domain-server-player-voice-state"
	banValue := 0
	if isBan {
		banValue = 1
	}
	requestData := map[string]any{
		"sid":    sid,
		"uid":    uid,
		"is_ban": banValue,
	}

	body, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.WebServerUrl+api, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(body), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var setResp CCVoiceSetPlayerStateResponse
	if err := json.Unmarshal(respBody, &setResp); err != nil {
		return nil, fmt.Errorf("解析语音玩家状态响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &setResp, nil
}
