package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// LoadDeveloperHomepageRequest 表示开发者主页查询请求。
type LoadDeveloperHomepageRequest struct {
	ID int64 `json:"id"`
}

// LoadDeveloperHomepageResponse 表示开发者主页查询响应。
type LoadDeveloperHomepageResponse struct {
	Response
	SummaryMD5 string                  `json:"summary_md5"`
	Entity     DeveloperHomepageEntity `json:"entity"`
}

// DeveloperHomepageEntity 表示开发者主页基础信息。
type DeveloperHomepageEntity struct {
	ShortIntro      string    `json:"short_intro"`
	FellowNum       Uncertain `json:"fellow_num"`
	DeveloperName   string    `json:"developer_name"`
	BackImage       string    `json:"back_image"`
	HeadImage       string    `json:"headimg"`
	Grade           Uncertain `json:"grade"`
	CurrentClass    Uncertain `json:"current_class"`
	CurrentLevel    Uncertain `json:"current_level"`
	ClassName       string    `json:"class_name"`
	AuthTag         *string   `json:"auth_tag"`
	CoreUID         Uncertain `json:"core_uid"`
	Nickname        string    `json:"nickname"`
	IsFellow        Uncertain `json:"is_fellow"`
	DeveloperInfoID Uncertain `json:"developer_info_id"`
}

// GetDeveloperUploadRequest 表示开发者动态列表请求。
type GetDeveloperUploadRequest struct {
	DeveloperInfoID int64 `json:"developer_info_id"`
	Length          int   `json:"length"`
	Offset          int   `json:"offset"`
}

// GetDeveloperUploadResponse 表示开发者动态列表响应。
type GetDeveloperUploadResponse struct {
	Response
	Entities []DeveloperUploadEntity `json:"entities"`
}

// DeveloperUploadEntity 表示开发者单条动态记录。
type DeveloperUploadEntity struct {
	DeveloperInfoID Uncertain `json:"developer_info_id"`
	IID             string    `json:"iid"`
	UpdateSummary   string    `json:"update_summary"`
	UpdateTime      Uncertain `json:"update_time"`
	CreateTime      Uncertain `json:"create_time"`
	GoodNum         Uncertain `json:"good_num"`
}

// LoadDeveloperNumberListRequest 表示开发者成员列表请求。
type LoadDeveloperNumberListRequest struct {
	Length          int   `json:"length"`
	DeveloperInfoID int64 `json:"developer_info_id"`
	Offset          int   `json:"offset"`
}

// LoadDeveloperNumberListResponse 表示开发者成员列表响应。
type LoadDeveloperNumberListResponse struct {
	Response
	Entities []DeveloperNumberListEntity `json:"entities"`
}

// DeveloperNumberListEntity 表示开发者成员条目。
type DeveloperNumberListEntity struct {
	UID                 Uncertain                `json:"uid"`
	DeveloperInfoID     Uncertain                `json:"developer_info_id"`
	IsCore              Uncertain                `json:"is_core"`
	IsDeveloper         Uncertain                `json:"is_developer"`
	DevelopPermission   Uncertain                `json:"develop_permission"`
	OnlinePermission    Uncertain                `json:"online_permission"`
	BetaLoginPermission Uncertain                `json:"beta_login_permission"`
	AuthStatus          Uncertain                `json:"auth_status"`
	Nickname            string                   `json:"nickname"`
	HeadImage           string                   `json:"headImage"`
	FrameID             string                   `json:"frame_id"`
	AdoptCommentCount   Uncertain                `json:"adopt_comment_count"`
	RechargeVIPInfo     DeveloperRechargeVIPInfo `json:"recharge_vip_info"`
	RechargeVIPLevel    Uncertain                `json:"recharge_vip_level"`
	GrowthLevel         Uncertain                `json:"growth_lv"`
}

// DeveloperRechargeVIPInfo 表示开发者成员的会员权益。
type DeveloperRechargeVIPInfo struct {
	Level1 Uncertain `json:"1"`
	Level5 Uncertain `json:"5"`
	Level6 Uncertain `json:"6"`
	Level7 Uncertain `json:"7"`
}

// LoadItemsByDeveloperInfoIDRequest 表示开发者作品列表请求。
type LoadItemsByDeveloperInfoIDRequest struct {
	SortType        *int  `json:"sort_type,omitempty"`
	ChannelID       int   `json:"channel_id"`
	Length          int   `json:"length"`
	DeveloperInfoID int64 `json:"developer_info_id"`
	Offset          int   `json:"offset"`
}

// LoadItemsByDeveloperInfoIDResponse 表示开发者作品列表响应。
type LoadItemsByDeveloperInfoIDResponse struct {
	Response
	Entities []DeveloperHomepageItem `json:"entities"`
	Total    Uncertain               `json:"total"`
}

// DeveloperHomepageItem 表示开发者主页下的单个资源。
type DeveloperHomepageItem struct {
	ItemID               string    `json:"item_id"`
	FirstType            Uncertain `json:"first_type"`
	SecondType           Uncertain `json:"second_type"`
	PicTagState          Uncertain `json:"pic_tag_state"`
	ResName              string    `json:"res_name"`
	ResSize              string    `json:"res_size"`
	ResMD5               string    `json:"res_md5"`
	Stars                Uncertain `json:"stars"`
	DownloadNum          string    `json:"download_num"`
	WeekDownloadNum      string    `json:"week_download_num"`
	Points               Uncertain `json:"points"`
	Diamond              Uncertain `json:"diamond"`
	ResVersion           Uncertain `json:"res_version"`
	IsItemTimeLimit      Uncertain `json:"is_item_time_limit"`
	ItemRemainTime       Uncertain `json:"item_remain_time"`
	BuyState             Uncertain `json:"buy_state"`
	GoodsState           Uncertain `json:"goods_state"`
	Status               Uncertain `json:"status"`
	SkinBodyType         Uncertain `json:"skin_body_type"`
	TitleImageURL        string    `json:"title_image_url"`
	TitleImageVersion    Uncertain `json:"title_image_version"`
	ResourcePacksVersion string    `json:"resource_packs_version"`
	BehaviorPacksVersion string    `json:"behavior_packs_version"`
	LobbyTag             []string  `json:"lobby_tag"`
	RelIID               Uncertain `json:"rel_iid"`
	IsCompetitive        Uncertain `json:"is_competitive"`
	AdvObtainNum         Uncertain `json:"adv_obtain_num"`
	Discount             Uncertain `json:"discount"`
	VIPDiscount          Uncertain `json:"vip_discount"`
	IsVIPBenefit         Uncertain `json:"is_vip_benefit"`
	PayChannel           string    `json:"pay_channel"`
	ProductID            string    `json:"product_id"`
	IsRecommend          Uncertain `json:"is_recommend"`
	RecInfo              []string  `json:"rec_info"`
	RemarkNum            Uncertain `json:"remark_num"`
	IsTop                Uncertain `json:"is_top"`
	IsJoint              Uncertain `json:"is_joint"`
	SellTags             []string  `json:"sell_tags"`
	RebateActivityID     string    `json:"rebate_activity_id"`
	IsEA                 Uncertain `json:"is_ea"`
	SeasonModID          string    `json:"season_mod_id"`
	EntityID             string    `json:"entity_id"`
	PersonaMTypeID       Uncertain `json:"persona_mtypeid"`
	PersonaSTypeID       Uncertain `json:"persona_stypeid"`
	IsSync               Uncertain `json:"is_sync"`
	RebateTag            Uncertain `json:"rebate_tag"`
	JellyID              string    `json:"jelly_id"`
}

// LoadDeveloperHomepage 获取开发者主页基础信息。
func (c *Client) LoadDeveloperHomepage(request LoadDeveloperHomepageRequest) (*LoadDeveloperHomepageResponse, error) {
	if request.ID <= 0 {
		return nil, fmt.Errorf("LoadDeveloperHomepage: id 必须大于 0")
	}

	api := "/pe-developer-homepage/load_developer_homepage/get"

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

	var result LoadDeveloperHomepageResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析开发者主页失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}

// GetDeveloperUpload 获取开发者动态列表。
func (c *Client) GetDeveloperUpload(request GetDeveloperUploadRequest) (*GetDeveloperUploadResponse, error) {
	if request.DeveloperInfoID <= 0 {
		return nil, fmt.Errorf("GetDeveloperUpload: developer_info_id 必须大于 0")
	}

	api := "/pe-developer-homepage/get-developer-upload"

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

	var result GetDeveloperUploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析开发者动态列表失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}

// LoadDeveloperNumberList 获取开发者成员列表。
func (c *Client) LoadDeveloperNumberList(request LoadDeveloperNumberListRequest) (*LoadDeveloperNumberListResponse, error) {
	if request.DeveloperInfoID <= 0 {
		return nil, fmt.Errorf("LoadDeveloperNumberList: developer_info_id 必须大于 0")
	}

	// 服务端路径中 develoer 为拼写错误，这里保持与实际接口一致。
	api := "/pe-developer-homepage/load_develoer_number_list/"

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

	var result LoadDeveloperNumberListResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析开发者成员列表失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}

// LoadItemsByDeveloperInfoID 获取开发者主页资源列表。
func (c *Client) LoadItemsByDeveloperInfoID(request LoadItemsByDeveloperInfoIDRequest) (*LoadItemsByDeveloperInfoIDResponse, error) {
	if request.DeveloperInfoID <= 0 {
		return nil, fmt.Errorf("LoadItemsByDeveloperInfoID: developer_info_id 必须大于 0")
	}

	api := "/pe-developer-homepage/load_items_by_developer_info_id"

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

	var result LoadItemsByDeveloperInfoIDResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析开发者资源列表失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}
