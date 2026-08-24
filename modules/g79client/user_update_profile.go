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
}

// UpdateProfile 批量更新账号资料（昵称/简介/头像/头像框/性别）。
// 接口路径: /pe-user-detail/update
func (c *Client) UpdateProfile(req *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("UpdateProfile: request 不能为空")
	}
	api := "/pe-user-detail/update"

	requestData := map[string]interface{}{}
	if req.Name != "" {
		requestData["name"] = req.Name
	}
	if req.Signature != "" {
		requestData["signature"] = req.Signature
	}
	if req.HeadImage != "" {
		requestData["head_image"] = req.HeadImage
	}
	if req.FrameID != "" {
		requestData["frame_id"] = req.FrameID
	}
	if req.Gender != "" {
		requestData["gender"] = req.Gender
	}
	if len(requestData) == 0 {
		return nil, fmt.Errorf("UpdateProfile: 至少填写一个要修改的字段")
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
	httpReq.Header.Set("user-id", c.UserID)
	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	httpReq.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result UpdateProfileResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 UpdateProfile 响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	if result.Code != 0 {
		return &result, fmt.Errorf("UpdateProfile 失败: code=%d msg=%s", result.Code, result.Message)
	}

	// 更新本地缓存
	if c.UserDetail != nil {
		if req.Name != "" {
			c.UserDetail.Name = req.Name
		}
		if req.Signature != "" {
			c.UserDetail.Signature = req.Signature
		}
		if req.HeadImage != "" {
			c.UserDetail.HeadImage = req.HeadImage
		}
		if req.FrameID != "" {
			c.UserDetail.FrameID = req.FrameID
		}
		if req.Gender != "" {
			c.UserDetail.Gender = req.Gender
		}
	}
	return &result, nil
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
