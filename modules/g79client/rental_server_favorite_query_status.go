package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// QueryRentalServerFavoriteStatusRequest 表示租赁服收藏状态请求。
type QueryRentalServerFavoriteStatusRequest struct {
	ServerID string `json:"server_id"`
}

// RentalServerFavoriteStatusEntity 表示租赁服收藏状态实体。
type RentalServerFavoriteStatusEntity struct {
	SID        Uncertain `json:"sid"`
	IsFavorite bool      `json:"is_favorite"`
}

// QueryRentalServerFavoriteStatusResponse 表示租赁服收藏状态响应。
type QueryRentalServerFavoriteStatusResponse struct {
	Response
	Entity RentalServerFavoriteStatusEntity `json:"entity"`
}

// QueryRentalServerFavoriteStatus 查询租赁服收藏状态。
func (c *Client) QueryRentalServerFavoriteStatus(request QueryRentalServerFavoriteStatusRequest) (*QueryRentalServerFavoriteStatusResponse, error) {
	request.ServerID = strings.TrimSpace(request.ServerID)
	if request.ServerID == "" {
		return nil, fmt.Errorf("QueryRentalServerFavoriteStatus: server_id 不能为空")
	}

	api := "/rental-server-favorite/query-status"

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

	var result QueryRentalServerFavoriteStatusResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析租赁服收藏状态响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}
