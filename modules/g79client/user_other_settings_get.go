package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

var defaultOtherUserSettingKeys = []string{
	"skin_type",
	"skin_data",
	"persona_data",
	"screen_config",
}

// GetOtherUserSettingListRequest 表示他人设置查询请求。
type GetOtherUserSettingListRequest struct {
	WithPetInfo bool     `json:"with_pet_info"`
	UserID      string   `json:"user_id"`
	Settings    []string `json:"settings"`
}

// GetOtherUserSettingListResponse 表示他人设置查询响应。
type GetOtherUserSettingListResponse struct {
	Response
	Entity OtherUserSettingListEntity `json:"entity"`
}

// OtherUserSettingListEntity 表示他人设置详情。
type OtherUserSettingListEntity struct {
	SkinType     OtherUserSkinType     `json:"skin_type"`
	SkinData     OtherUserSkinData     `json:"skin_data"`
	PersonaData  OtherUserPersonaData  `json:"persona_data"`
	ScreenConfig OtherUserScreenConfig `json:"screen_config"`
	PetInfo      *OtherUserPetInfo     `json:"pet_info,omitempty"`
}

// OtherUserSkinType 表示皮肤类型设置。
type OtherUserSkinType struct {
	Type string `json:"type"`
}

// OtherUserSkinData 表示皮肤配置。
type OtherUserSkinData struct {
	ItemID     string `json:"item_id"`
	IsCharPack bool   `json:"is_char_pack"`
	IsSlim     bool   `json:"is_slim"`
}

// OtherUserPersonaData 表示捏脸配置。
type OtherUserPersonaData struct{}

// OtherUserScreenConfig 表示展示位配置列表。
type OtherUserScreenConfig []OtherUserScreenConfigEntry

// OtherUserScreenConfigEntry 表示单个展示位配置。
type OtherUserScreenConfigEntry struct {
	Slot          int       `json:"slot"`
	ItemID        string    `json:"item_id"`
	OutfitLevel   Uncertain `json:"outfit_level"`
	BehaviourUUID string    `json:"behaviour_uuid"`
	EffectMTypeID Uncertain `json:"effect_mtypeid"`
	EffectSTypeID Uncertain `json:"effect_stypeid"`
}

func (c *OtherUserScreenConfig) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || string(b) == "{}" {
		*c = OtherUserScreenConfig{}
		return nil
	}

	type rawEntry OtherUserScreenConfigEntry
	rawMap := map[string]rawEntry{}
	if err := json.Unmarshal(b, &rawMap); err != nil {
		return err
	}

	keys := make([]string, 0, len(rawMap))
	for key := range rawMap {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		left, leftErr := strconv.Atoi(keys[i])
		right, rightErr := strconv.Atoi(keys[j])
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		return keys[i] < keys[j]
	})

	result := make(OtherUserScreenConfig, 0, len(rawMap))
	for _, key := range keys {
		entry := OtherUserScreenConfigEntry(rawMap[key])
		slot, err := strconv.Atoi(key)
		if err == nil {
			entry.Slot = slot
		}
		result = append(result, entry)
	}

	*c = result
	return nil
}

func (c OtherUserScreenConfig) MarshalJSON() ([]byte, error) {
	type rawEntry OtherUserScreenConfigEntry
	rawMap := make(map[string]rawEntry, len(c))
	for _, entry := range c {
		rawMap[strconv.Itoa(entry.Slot)] = rawEntry(entry)
	}
	return json.Marshal(rawMap)
}

// OtherUserPetInfo 表示宠物配置。
type OtherUserPetInfo struct {
	PetNum    Uncertain            `json:"pet_num"`
	PetName   string               `json:"pet_name"`
	SkinInfo  OtherUserPetSkinInfo `json:"skin_info"`
	ActionIDs []Uncertain          `json:"action_ids"`
}

// OtherUserPetSkinInfo 表示宠物皮肤详情。
type OtherUserPetSkinInfo struct {
	IsActivity  Uncertain `json:"is_activity"`
	Name        string    `json:"name"`
	SizeX       Uncertain `json:"size_x"`
	SizeY       Uncertain `json:"size_y"`
	ShadowX     Uncertain `json:"shadow_x"`
	ShadowY     Uncertain `json:"shadow_y"`
	Rarity      Uncertain `json:"rarity"`
	Material    string    `json:"material"`
	Icon        string    `json:"icon"`
	PreviewIcon string    `json:"preview_icon"`
	Score       Uncertain `json:"score"`
	Type        Uncertain `json:"type"`
	Desc        string    `json:"desc"`
	BuyNum      Uncertain `json:"buy_num"`
	CategoryID  Uncertain `json:"category_id"`
	OnlineTime  string    `json:"online_time"`
UpdateTime  string    `json:"update_time"`
	OffsetX     Uncertain `json:"offset_x"`
	OffsetY     Uncertain `json:"offset_y"`
	Scale       Uncertain `json:"scale"`
	SkinID      Uncertain `json:"skin_id"`
	Effect      string    `json:"effect"`
}

// GetOtherUserSettingList 获取指定玩家的设置列表。
func (c *Client) GetOtherUserSettingList(request GetOtherUserSettingListRequest) (*GetOtherUserSettingListResponse, error) {
	if strings.TrimSpace(request.UserID) == "" {
		return nil, fmt.Errorf("GetOtherUserSettingList: user_id 不能为空")
	}
	if len(request.Settings) == 0 {
		request.Settings = append([]string(nil), defaultOtherUserSettingKeys...)
	}

	api := "/pe-get-other-user-setting-list"

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

	var result GetOtherUserSettingListResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析他人设置列表失败: %v, 响应内容: %s", err, string(respBody))
	}

	return &result, nil
}
