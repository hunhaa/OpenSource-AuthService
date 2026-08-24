package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// UpdateProfileRequest 用户资料更新请求。
// 所有字段可选，留空表示不修改。
type UpdateProfileRequest struct {
	// 昵称（也可用 UpdateNickname 单独改）
	Name string `json:"name,omitempty"`
	// 简介 / 个性签名
	Signature string `json:"signature,omitempty"`
	// 头像 ID（一般是素材 item_id；留空表示不改）
	HeadImage string `json:"head_image,omitempty"`
	// 头像框
	FrameID string `json:"frame_id,omitempty"`
	// 性别 "male" / "female"
	Gender string `json:"gender,omitempty"`
}

type UpdateProfileResponse struct {
	Response
	AppliedNickname string `json:"applied_nickname,omitempty"`
	AppliedPersona  string `json:"applied_persona,omitempty"`
}

// UpdateProfile 批量更新账号资料（昵称/简介/头像/头像框/性别）。
// 网易 G79 没有统一的 /pe-user-detail/update 接口，各字段分属不同端点：
//   - 昵称 → /pe-nickname-setting/update（通过 UpdateNickname）
//   - 简介/头像/头像框/性别 → /pe-set-user-setting-list 的 data.persona_data
func (c *Client) UpdateProfile(req *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("UpdateProfile: request 不能为空")
	}

	hasName := strings.TrimSpace(req.Name) != ""
	hasPersona := strings.TrimSpace(req.Signature) != "" ||
		strings.TrimSpace(req.HeadImage) != "" ||
		strings.TrimSpace(req.FrameID) != "" ||
		strings.TrimSpace(req.Gender) != ""
	if !hasName && !hasPersona {
		return nil, fmt.Errorf("UpdateProfile: 至少填写一个要修改的字段")
	}

	result := &UpdateProfileResponse{}
	var lastErr error

	// ── 1. 昵称：走独立的 /pe-nickname-setting/update ──
	if hasName {
		name := strings.TrimSpace(req.Name)
		if err := c.UpdateNickname(name); err != nil {
			lastErr = fmt.Errorf("更新昵称失败: %w", err)
		} else {
			result.AppliedNickname = name
			if c.UserDetail != nil {
				c.UserDetail.Name = name
			}
		}
	}

	// ── 2. 其他资料字段：走 /pe-set-user-setting-list 的 persona_data ──
	if hasPersona {
		api := "/pe-set-user-setting-list"
		persona := map[string]any{}
		if strings.TrimSpace(req.Signature) != "" {
			persona["signature"] = strings.TrimSpace(req.Signature)
		}
		if strings.TrimSpace(req.HeadImage) != "" {
			persona["head_image"] = strings.TrimSpace(req.HeadImage)
		}
		if strings.TrimSpace(req.FrameID) != "" {
			persona["frame_id"] = strings.TrimSpace(req.FrameID)
		}
		if strings.TrimSpace(req.Gender) != "" {
			persona["gender"] = strings.TrimSpace(req.Gender)
		}
		requestData := map[string]interface{}{
			"data": map[string]any{
				"persona_data": persona,
			},
		}

		jsonData, err := json.Marshal(requestData)
		if err != nil {
			if lastErr == nil {
				lastErr = err
			}
		} else {
			httpReq, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
			if err != nil {
				if lastErr == nil {
					lastErr = fmt.Errorf("构建 persona 请求失败: %w", err)
				}
			} else {
				httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
				httpReq.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
				httpReq.Header.Set("user-id", c.UserID)
				token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
				httpReq.Header.Set("user-token", token)

				resp, err := c.httpClient.Do(httpReq)
				if err != nil {
					if lastErr == nil {
						lastErr = fmt.Errorf("发送 persona 请求失败: %w", err)
					}
				} else {
					respBody, _ := readResponseBody(resp)
					_ = resp.Body.Close()
					var setResp Response
					if jerr := json.Unmarshal(respBody, &setResp); jerr != nil {
						if lastErr == nil {
							lastErr = fmt.Errorf("解析 persona 响应失败: %v, 内容: %s", jerr, trimForErr(respBody))
						}
					} else if setResp.Code != 0 {
						if lastErr == nil {
							lastErr = fmt.Errorf("更新 persona 资料失败: code=%d msg=%s", setResp.Code, setResp.Message)
						}
					} else {
						if personaBytes, perr := json.Marshal(persona); perr == nil {
							result.AppliedPersona = string(personaBytes)
						}
						result.Code = setResp.Code
						result.Message = setResp.Message
						if c.UserDetail != nil {
							if v, ok := persona["signature"].(string); ok {
								c.UserDetail.Signature = v
							}
							if v, ok := persona["head_image"].(string); ok {
								c.UserDetail.HeadImage = v
							}
							if v, ok := persona["frame_id"].(string); ok {
								c.UserDetail.FrameID = v
							}
							if v, ok := persona["gender"].(string); ok {
								c.UserDetail.Gender = v
							}
						}
					}
				}
			}
		}
	}

	if lastErr != nil {
		if result.Code == 0 && (result.AppliedNickname != "" || result.AppliedPersona != "") {
			// 有部分成功：返回已应用的信息 + 错误供上层感知
			return result, lastErr
		}
		return nil, lastErr
	}
	return result, nil
}

// UpdateSignature 只改简介（个性签名），是 UpdateProfile 的简写版。
func (c *Client) UpdateSignature(signature string) error {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return fmt.Errorf("UpdateSignature: 简介不能为空")
	}
	_, err := c.UpdateProfile(&UpdateProfileRequest{Signature: signature})
	return err
}

// UpdateHeadImage 只改头像（item_id），是 UpdateProfile 的简写版。
func (c *Client) UpdateHeadImage(headImageID string) error {
	headImageID = strings.TrimSpace(headImageID)
	if headImageID == "" {
		return fmt.Errorf("UpdateHeadImage: 头像 ID 不能为空")
	}
	_, err := c.UpdateProfile(&UpdateProfileRequest{HeadImage: headImageID})
	return err
}

