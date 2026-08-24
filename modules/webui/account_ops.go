package webui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Yeah114/g79client"
	"github.com/gin-gonic/gin"
)

// ---------- 请求结构体 ----------

type AccountUpdateProfileReq struct {
	Token     string `json:"token"`
	Name      string `json:"name"`       // 昵称
	Signature string `json:"signature"`  // 简介/个性签名
	HeadImage string `json:"head_image"` // 头像ID
	FrameID   string `json:"frame_id"`   // 头像框
	Gender    string `json:"gender"`     // 性别
}

type AccountChangeSkinReq struct {
	Token  string `json:"token"`
	ItemID string `json:"item_id"` // 皮肤 item_id (从商城 item 列表拿)
}

type AccountSendMomentReq struct {
	Token   string                     `json:"token"`
	Content string                     `json:"content"`
	Opts    *g79client.SendMomentOptions `json:"opts,omitempty"`
}

type AccountGetUserDetailReq struct {
	Token string `json:"token"`
}

type AccountVitalityCheckInReq struct {
	Token string `json:"token"`
}

// ---------- 路由（在 RegisterRoutes 内调用 g 注册）----------
// 为保持 webui.go 结构清晰，这里单独提供一个函数，由 webui.go 里的 RegisterRoutes 调用。
func RegisterAccountOpsRoutes(g *gin.RouterGroup) {
	g.POST("/account/profile", HandleAccountProfile)       // 查看资料
	g.POST("/account/update",  HandleAccountUpdateProfile) // 改昵称/简介/头像/头像框/性别
	g.POST("/account/skin",    HandleAccountChangeSkin)    // 换皮肤
	g.POST("/account/moment",  HandleAccountSendMoment)    // 发动态
	g.POST("/account/vitality",HandleAccountVitality)      // 查活力 + 签到
}

// ---------- handlers ----------

// HandleAccountProfile 查看账号资料（名称/简介/等级/活力/VIP/绑定状态等）
func HandleAccountProfile(c *gin.Context) {
	var req AccountGetUserDetailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}
	// 拿详情
	detail, err := s.Client.GetUserDetail()
	if err != nil {
		fail(c, 500, "GetUserDetail 失败: %v", err)
		return
	}
	if detail.Code != 0 {
		fail(c, 422, "查询资料失败 code=%d msg=%s", detail.Code, detail.Message)
		return
	}
	// 拿设置（皮肤/展示位）
	settings, _ := s.Client.GetUserSettingList()
	// 拿活力/成长
	currency, _ := s.Client.GetCurrencyOnline()
	growth, _ := s.Client.GetDailyGrowth()

	e := detail.Entity
	ok(c, gin.H{
		"user_id":   s.UserID,
		"nickname":  e.Name,
		"signature": e.Signature,
		"head_image": e.HeadImage,
		"frame_id":  e.FrameID,
		"gender":    e.Gender,
		"account":   e.Account,
		"level":     e.Level.String(),
		"score":     e.Score.String(),
		"skin_number":   e.SkinNumber.String(),
		"cape_number":   e.CapeNumber.String(),
		"remain_revise_name_cnt": e.RemainReviseNameCnt.String(),
		"nickname_free": e.NicknameFree.String(),
		"realname_status": e.RealnameStatus.String(),
		"need_phone_bind": e.NeedPhoneBind,
		"is_phone_bind":   e.IsPhoneBind.String(),
		"is_bind":         e.IsBind,
		"wechat_info":     e.WechatInfo,
		"public_flag":     e.PublicFlag,
		"is_vip":          e.IsVIP,
		"is_expr_vip":     e.IsExprVIP,
		"recharge_vip_level": e.RechargeVIPLevel.String(),
		"register_time":   e.RegisterTime.String(),
		"login_time":      e.LoginTime.String(),
		"settings": func() any {
			if settings == nil { return nil }
			return settings.Entity
		}(),
		"vitality_rest_sec": func() int {
			if currency == nil { return 0 }
			return currency.Entity.RestCurrencyTime
		}(),
		"vitality_date": func() string {
			if currency == nil { return "" }
			return currency.Entity.Date
		}(),
		"daily_xp_online": func() int {
			if growth == nil { return 0 }
			return growth.Entity.XPFromOnline
		}(),
		"daily_xp_recharge": func() int {
			if growth == nil { return 0 }
			return growth.Entity.XPFromRecharge
		}(),
	})
}

// HandleAccountUpdateProfile 改昵称/简介/头像/头像框/性别
func HandleAccountUpdateProfile(c *gin.Context) {
	var req AccountUpdateProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}
	if strings.TrimSpace(req.Name) == "" &&
		strings.TrimSpace(req.Signature) == "" &&
		strings.TrimSpace(req.HeadImage) == "" &&
		strings.TrimSpace(req.FrameID) == "" &&
		strings.TrimSpace(req.Gender) == "" {
		fail(c, 400, "至少填写一个要修改的字段 (name/signature/head_image/frame_id/gender)")
		return
	}

	greq := &g79client.UpdateProfileRequest{
		Name:      strings.TrimSpace(req.Name),
		Signature: req.Signature,
		HeadImage: strings.TrimSpace(req.HeadImage),
		FrameID:   strings.TrimSpace(req.FrameID),
		Gender:    strings.TrimSpace(req.Gender),
	}
	resp, err := s.Client.UpdateProfile(greq)
	if err != nil {
		fail(c, 500, "更新资料失败: %v", err)
		return
	}
	// 更新本地缓存的 Nickname
	if greq.Name != "" {
		s.mu.Lock()
		s.Nickname = greq.Name
		s.mu.Unlock()
	}
	ok(c, gin.H{
		"code":    resp.Code,
		"msg":     resp.Message,
		"applied": greq,
	})
}

// HandleAccountChangeSkin 更换皮肤
func HandleAccountChangeSkin(c *gin.Context) {
	var req AccountChangeSkinReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}
	itemID := strings.TrimSpace(req.ItemID)
	if itemID == "" {
		fail(c, 400, "item_id 不能为空")
		return
	}
	if err := s.Client.ChangeSkin(itemID); err != nil {
		fail(c, 500, "更换皮肤失败: %v", err)
		return
	}
	ok(c, gin.H{"item_id": itemID, "msg": "皮肤已更换"})
}

// HandleAccountSendMoment 发送动态
func HandleAccountSendMoment(c *gin.Context) {
	var req AccountSendMomentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		fail(c, 400, "content 动态正文不能为空")
		return
	}
	opts := req.Opts
	if opts == nil {
		opts = &g79client.SendMomentOptions{CommentAuth: 1}
	}
	resp, err := s.Client.SendMoment(content, opts)
	if err != nil {
		fail(c, 500, "发送动态失败: %v", err)
		return
	}
	if resp.Code != 0 {
		fail(c, 422, "发送动态失败 code=%d msg=%s", resp.Code, resp.Message)
		return
	}
	ok(c, gin.H{
		"msg_id": resp.Entity.Data.MsgID,
		"status": resp.Entity.Status,
	})
}

// HandleAccountVitality 签到 + 查询活力（实际上 get-currency-online 既是心跳拿活力也是查询）
func HandleAccountVitality(c *gin.Context) {
	var req AccountVitalityCheckInReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}

	var (
		lastErr error
		restSec int
		dateStr string
		xpOnline, xpRecharge int
	)

	// 先查在线活力
	if cur, err := s.Client.GetCurrencyOnline(); err == nil && cur.Code == 0 {
		restSec = cur.Entity.RestCurrencyTime
		dateStr = cur.Entity.Date
	} else if err != nil {
		lastErr = fmt.Errorf("查询活力失败: %w", err)
	} else {
		lastErr = fmt.Errorf("查询活力 code=%d msg=%s", cur.Code, cur.Message)
	}

	// 查每日成长
	if g, err := s.Client.GetDailyGrowth(); err == nil && g.Code == 0 {
		xpOnline = g.Entity.XPFromOnline
		xpRecharge = g.Entity.XPFromRecharge
	}

	c.JSON(http.StatusOK, R{
		OK: lastErr == nil,
		Msg: func() string {
			if lastErr != nil {
				return lastErr.Error()
			}
			return "活力查询成功（GetCurrencyOnline 即签到心跳）"
		}(),
		Data: gin.H{
			"rest_seconds":      restSec,
			"rest_minutes":      restSec / 60,
			"rest_hhmm":         fmt.Sprintf("%02d:%02d", restSec/3600, (restSec%3600)/60),
			"date":              dateStr,
			"daily_xp_online":   xpOnline,
			"daily_xp_recharge": xpRecharge,
			"checkin_tip":       "已调用 /get-currency-online 作为心跳签到，活力时长会随在线时间累积。",
		},
	})
}
