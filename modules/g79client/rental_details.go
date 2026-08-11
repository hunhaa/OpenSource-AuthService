package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GetRentalServerDetailsRequest 表示租赁服详情请求。
type GetRentalServerDetailsRequest struct {
	ServerID string `json:"server_id"`
}

// RentalServerDetailsEntity 表示租赁服详情实体。
type RentalServerDetailsEntity struct {
	EntityID               string    `json:"entity_id"`
	WorldID                string    `json:"world_id"`
	OwnerID                string    `json:"owner_id"`
	Name                   string    `json:"name"`
	BriefSummary           string    `json:"brief_summary"`
	IconIndex              Uncertain `json:"icon_index"`
	BeginTime              Uncertain `json:"begin_time"`
	McVersion              string    `json:"mc_version"`
	Capacity               Uncertain `json:"capacity"`
	ActiveComponents       []any     `json:"active_components"`
	UpdateActiveComponents []any     `json:"update_active_components"`
	Status                 Uncertain `json:"status"`
	Visibility             Uncertain `json:"visibility"`
	PlayerCount            Uncertain `json:"player_count"`
	LikeNum                Uncertain `json:"like_num"`
	ServerType             string    `json:"server_type"`
	HasPwd                 Uncertain `json:"has_pwd"`
	ImageURL               string    `json:"image_url"`
	MinLevel               Uncertain `json:"min_level"`
	ServerName             string    `json:"server_name"`
	PVP                    bool      `json:"pvp"`
}

// GetRentalServerDetailsResponse 表示租赁服详情响应。
type GetRentalServerDetailsResponse struct {
	Response
	Entity RentalServerDetailsEntity `json:"entity"`
}

// GetRentalServerDetails 获取租赁服详情。
func (c *Client) GetRentalServerDetails(serverID string) (*GetRentalServerDetailsResponse, error) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return nil, fmt.Errorf("GetRentalServerDetails: server_id 不能为空")
	}

	api := "/rental-server-details/get"

	request := GetRentalServerDetailsRequest{
		ServerID: serverID,
	}

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

	var detailsResp GetRentalServerDetailsResponse
	if err := json.Unmarshal(respBody, &detailsResp); err != nil {
		return nil, fmt.Errorf("解析租赁服详情响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &detailsResp, nil
}
