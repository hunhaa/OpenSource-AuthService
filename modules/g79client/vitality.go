package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	vitalityGetCurrencyOnlinePath = "/get-currency-online"
	vitalityDailyGrowthPath       = "/pe-get-daily-growth-info"
)

// GetCurrencyOnlineResponse 表示 /get-currency-online 的响应。
type GetCurrencyOnlineResponse struct {
	Response
	Entity GetCurrencyOnlineEntity `json:"entity"`
}

// GetCurrencyOnlineEntity 描述活力在线计时状态。
type GetCurrencyOnlineEntity struct {
	RestCurrencyTime int    `json:"rest_currency_time"`
	Date             string `json:"date"`
}

// DailyGrowthResponse 表示 /pe-get-daily-growth-info 的响应。
type DailyGrowthResponse struct {
	Response
	Entity DailyGrowthEntity `json:"entity"`
}

// DailyGrowthEntity 描述今日成长经验来源。
type DailyGrowthEntity struct {
	XPFromOnline   int `json:"1"`
	XPFromRecharge int `json:"2"`
}

// GetCurrencyOnline 请求服务端累计在线活力时长。
func (c *Client) GetCurrencyOnline() (*GetCurrencyOnlineResponse, error) {
	var result GetCurrencyOnlineResponse
	if err := c.postVitality(vitalityGetCurrencyOnlinePath, "{}", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDailyGrowth 查询今日在线与充值获得的成长经验。
func (c *Client) GetDailyGrowth() (*DailyGrowthResponse, error) {
	var result DailyGrowthResponse
	if err := c.postVitality(vitalityDailyGrowthPath, "{}", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) postVitality(api, body string, out any) error {
	if c == nil {
		return fmt.Errorf("g79client: client 不能为空")
	}
	if strings.TrimSpace(c.ReleaseJSON.ApiGatewayUrl) == "" {
		return fmt.Errorf("g79client: ApiGatewayUrl 不能为空")
	}

	req, err := http.NewRequest(http.MethodPost, c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, body, c.UserToken))

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("解析活力接口响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return nil
}
