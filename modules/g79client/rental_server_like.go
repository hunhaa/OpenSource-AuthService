package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GetRentalServerLikeRequest 表示租赁服点赞状态请求。
type GetRentalServerLikeRequest struct {
	ServerID string `json:"server_id"`
}

// RentalServerLikeEntity 表示租赁服点赞状态实体。
type RentalServerLikeEntity struct {
	EntityID string `json:"entity_id"`
	IsLike   bool   `json:"is_like"`
}

// GetRentalServerLikeResponse 表示租赁服点赞状态响应。
type GetRentalServerLikeResponse struct {
	Response
	Entity RentalServerLikeEntity `json:"entity"`
}

// UpdateRentalServerLikeRequest changes the current user's like state for a
// rental server. The service uses an integer instead of a JSON boolean.
type UpdateRentalServerLikeRequest struct {
	ServerID string `json:"server_id"`
	IsLike   int    `json:"is_like"`
}

type UpdateRentalServerLikeResponse struct {
	Response
	Entity RentalServerLikeEntity `json:"entity"`
}

// GetRentalServerLike 获取租赁服点赞状态。
func (c *Client) GetRentalServerLike(request GetRentalServerLikeRequest) (*GetRentalServerLikeResponse, error) {
	request.ServerID = strings.TrimSpace(request.ServerID)
	if request.ServerID == "" {
		return nil, fmt.Errorf("GetRentalServerLike: server_id 不能为空")
	}

	api := "/rental-server-like/get"

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
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

	var result GetRentalServerLikeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析租赁服点赞状态响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}

// UpdateRentalServerLike sets a rental server like state. It uses the same
// client HTTP transport as authentication and all other Client operations.
func (c *Client) UpdateRentalServerLike(serverID string, isLike bool) (*UpdateRentalServerLikeResponse, error) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return nil, fmt.Errorf("UpdateRentalServerLike: server_id cannot be empty")
	}
	isLikeValue := 0
	if isLike {
		isLikeValue = 1
	}
	api := "/rental-server-like/update"
	jsonData, err := json.Marshal(UpdateRentalServerLikeRequest{ServerID: serverID, IsLike: isLikeValue})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, string(jsonData), c.UserToken))

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	var result UpdateRentalServerLikeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode rental server like response: %w", err)
	}
	return &result, nil
}
