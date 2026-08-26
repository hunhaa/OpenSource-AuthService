package usercenter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Yeah114/FunAuth/internal/db"
)

// ---------- 用户管理 ----------

// HandleAdminListUsers GET /api/usercenter/admin/users
func HandleAdminListUsers(c *gin.Context) {
	page, size := parsePage(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	banned := c.Query("banned")
	tx := db.DB.Model(&db.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("username LIKE ? OR nickname LIKE ? OR uuid LIKE ?", like, like, like)
	}
	if banned == "1" || banned == "true" {
		tx = tx.Where("is_banned = 1")
	} else if banned == "0" || banned == "false" {
		tx = tx.Where("is_banned = 0")
	}
	tx = tx.Order("created_at DESC, uuid DESC")
	var users []db.User
	total, err := paginate(tx.Session(&gorm.Session{}), &users, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	// 附加：身份组 + 钱包余额
	out := make([]gin.H, 0, len(users))
	uuids := make([]string, 0, len(users))
	for _, u := range users {
		uuids = append(uuids, u.UUID)
	}
	walletMap := map[string]int64{}
	if len(uuids) > 0 {
		var ws []db.Wallet
		db.DB.Where("user_uuid IN ?", uuids).Find(&ws)
		for _, w := range ws {
			walletMap[w.UserUUID] = w.Balance
		}
	}
	for _, u := range users {
		groups, _ := userGroupInfos(u.UUID)
		out = append(out, gin.H{
			"uuid":            u.UUID,
			"username":        u.Username,
			"nickname":        u.Nickname,
			"avatar_url":      u.AvatarURL,
			"bio":             u.Bio,
			"quota":           u.Quota,
			"times":           u.Times,
			"balance":         walletMap[u.UUID],
			"is_banned":       u.IsBanned,
			"isX19":           u.IsX19,
			"nickname_prefix":  u.NicknamePrefix,
			"end_time":        u.EndTime,
			"created_at":      u.CreatedAt,
			"groups":          groups,
		})
	}
	ok(c, pageResult(out, total, page, size))
}

type adminUpdateUserReq struct {
	EndTime        *time.Time `json:"end_time,omitempty"`
	IsBanned       *bool      `json:"is_banned,omitempty"`
	NicknamePrefix *string    `json:"nickname_prefix,omitempty"`
	Nickname       *string     `json:"nickname,omitempty"`
	AvatarURL      *string    `json:"avatar_url,omitempty"`
	Bio            *string    `json:"bio,omitempty"`
}

// HandleAdminUpdateUser PUT /api/usercenter/admin/users/:uuid
func HandleAdminUpdateUser(c *gin.Context) {
	uuid := c.Param("uuid")
	var req adminUpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	updates := map[string]interface{}{}
	if req.EndTime != nil {
		updates["end_time"] = req.EndTime
	}
	if req.IsBanned != nil {
		updates["is_banned"] = *req.IsBanned
	}
	if req.NicknamePrefix != nil {
		updates["nickname_prefix"] = truncate(*req.NicknamePrefix, 50)
	}
	if req.Nickname != nil {
		updates["nickname"] = truncate(*req.Nickname, 64)
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = truncate(*req.AvatarURL, 512)
	}
	if req.Bio != nil {
		updates["bio"] = truncate(*req.Bio, 1024)
	}
	if len(updates) == 0 {
		fail(c, http.StatusBadRequest, "没有可更新字段")
		return
	}
	updates["updated_at"] = time.Now()
	if err := db.UpdateUser(uuid, updates); err != nil {
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	user, _ := db.GetUserByUUID(uuid)
	groups, _ := userGroupInfos(uuid)
	ok(c, publicUserView(user, groups))
}

type adjustReq struct {
	Amount int64  `json:"amount" binding:"required"`
	Type   string `json:"type" binding:"required"`
	Remark string `json:"remark,omitempty"`
}

// HandleAdminAdjustBalance POST /api/usercenter/admin/users/:uuid/adjust-balance
// type: admin_adjust | admin_set；amount 单位为分（admin_set 时直接覆盖）
func HandleAdminAdjustBalance(c *gin.Context) {
	uuid := c.Param("uuid")
	var req adjustReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Type != "admin_adjust" && req.Type != "admin_set" {
		fail(c, http.StatusBadRequest, "type 必须是 admin_adjust 或 admin_set")
		return
	}
	if req.Type == "admin_adjust" && req.Amount == 0 {
		fail(c, http.StatusBadRequest, "amount 不能为 0")
		return
	}

	var finalBalance int64
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		wallet, err := db.EnsureUserWallet(tx, uuid)
		if err != nil {
			return err
		}
		if err := tx.Clauses(forUpdate()).Where("user_uuid = ?", uuid).First(wallet).Error; err != nil {
			return err
		}
		var delta int64
		if req.Type == "admin_set" {
			delta = req.Amount - wallet.Balance
			finalBalance = req.Amount
		} else {
			delta = req.Amount
			finalBalance = wallet.Balance + req.Amount
			if finalBalance < 0 {
				return fmt.Errorf("余额不足")
			}
		}
		if err := tx.Model(&db.Wallet{}).Where("user_uuid = ?", uuid).
			Update("balance", finalBalance).Error; err != nil {
			return err
		}
		return tx.Create(&db.WalletTransaction{
			UserUUID: uuid,
			Amount:   delta,
			Type:     req.Type,
			Remark:   req.Remark,
		}).Error
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "余额不足") {
			fail(c, http.StatusBadRequest, msg)
		} else {
			fail(c, http.StatusInternalServerError, "调整失败")
		}
		return
	}
	ok(c, gin.H{"balance": finalBalance})
}

// HandleAdminAdjustQuota POST /api/usercenter/admin/users/:uuid/adjust-quota
func HandleAdminAdjustQuota(c *gin.Context) {
	uuid := c.Param("uuid")
	var req adjustReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Type != "admin_adjust" && req.Type != "admin_set" {
		fail(c, http.StatusBadRequest, "type 必须是 admin_adjust 或 admin_set")
		return
	}
	if req.Type == "admin_adjust" && req.Amount == 0 {
		fail(c, http.StatusBadRequest, "amount 不能为 0")
		return
	}

	var finalQuota int64
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		var user db.User
		if err := tx.Clauses(forUpdate()).Where("uuid = ?", uuid).First(&user).Error; err != nil {
			return err
		}
		var delta int64
		if req.Type == "admin_set" {
			delta = req.Amount - user.Quota
			finalQuota = req.Amount
		} else {
			delta = req.Amount
			finalQuota = user.Quota + req.Amount
			if finalQuota < 0 {
				return fmt.Errorf("额度不足")
			}
		}
		if err := tx.Model(&db.User{}).Where("uuid = ?", uuid).Update("quota", finalQuota).Error; err != nil {
			return err
		}
		return tx.Create(&db.QuotaTransaction{
			UserUUID: uuid,
			Amount:   delta,
			Type:     req.Type,
			Remark:   req.Remark,
		}).Error
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "额度不足") {
			fail(c, http.StatusBadRequest, msg)
		} else {
			fail(c, http.StatusInternalServerError, "调整失败")
		}
		return
	}
	ok(c, gin.H{"quota": finalQuota})
}

// HandleAdminAdjustTimes POST /api/usercenter/admin/users/:uuid/adjust-times
func HandleAdminAdjustTimes(c *gin.Context) {
	uuid := c.Param("uuid")
	var req adjustReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Type != "admin_adjust" && req.Type != "admin_set" {
		fail(c, http.StatusBadRequest, "type 必须是 admin_adjust 或 admin_set")
		return
	}
	if req.Type == "admin_adjust" && req.Amount == 0 {
		fail(c, http.StatusBadRequest, "amount 不能为 0")
		return
	}

	var finalTimes int64
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		var user db.User
		if err := tx.Clauses(forUpdate()).Where("uuid = ?", uuid).First(&user).Error; err != nil {
			return err
		}
		var delta int64
		if req.Type == "admin_set" {
			delta = req.Amount - user.Times
			finalTimes = req.Amount
		} else {
			delta = req.Amount
			finalTimes = user.Times + req.Amount
			if finalTimes < 0 {
				return fmt.Errorf("次数不足")
			}
		}
		if err := tx.Model(&db.User{}).Where("uuid = ?", uuid).Update("times", finalTimes).Error; err != nil {
			return err
		}
		return tx.Create(&db.TimesTransaction{
			UserUUID: uuid,
			Amount:   delta,
			Type:     req.Type,
			Remark:   req.Remark,
		}).Error
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "次数不足") {
			fail(c, http.StatusBadRequest, msg)
		} else {
			fail(c, http.StatusInternalServerError, "调整失败")
		}
		return
	}
	ok(c, gin.H{"times": finalTimes})
}

// ---------- 身份组管理 ----------

// HandleAdminListGroups GET /api/usercenter/admin/groups
func HandleAdminListGroups(c *gin.Context) {
	var groups []db.Group
	db.DB.Order("sort_order ASC, id ASC").Find(&groups)
	// 统计每组用户数
	out := make([]gin.H, 0, len(groups))
	gids := make([]uint, 0, len(groups))
	for _, g := range groups {
		gids = append(gids, g.ID)
	}
	countMap := map[uint]int64{}
	if len(gids) > 0 {
		type cnt struct {
			GroupID uint
			N       int64
		}
		var rows []cnt
		db.DB.Model(&db.UserGroup{}).Select("group_id, COUNT(*) as n").
			Where("group_id IN ?", gids).
			Where("expires_at IS NULL OR expires_at >= ?", time.Now()).
			Group("group_id").Scan(&rows)
		for _, r := range rows {
			countMap[r.GroupID] = r.N
		}
	}
	for _, g := range groups {
		out = append(out, gin.H{
			"id":          g.ID,
			"name":        g.Name,
			"permissions": ParseStringArray(g.Permissions),
			"description": g.Description,
			"is_default":  g.IsDefault,
			"sort_order":  g.SortOrder,
			"member_count": countMap[g.ID],
			"created_at":  g.CreatedAt,
		})
	}
	ok(c, out)
}

type groupReq struct {
	Name        string   `json:"name" binding:"required"`
	Permissions []string `json:"permissions,omitempty"`
	Description string  `json:"description,omitempty"`
	IsDefault   bool     `json:"is_default,omitempty"`
	SortOrder   int      `json:"sort_order,omitempty"`
}

// HandleAdminCreateGroup POST /api/usercenter/admin/groups
func HandleAdminCreateGroup(c *gin.Context) {
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Permissions == nil {
		req.Permissions = []string{}
	}
	perms, _ := json.Marshal(req.Permissions)
	// 切换默认组：若新建为默认，先把其它默认组取消
	if req.IsDefault {
		db.DB.Model(&db.Group{}).Where("is_default = 1").Update("is_default", false)
	}
	g := db.Group{
		Name:        req.Name,
		Permissions: string(perms),
		Description: req.Description,
		IsDefault:   req.IsDefault,
		SortOrder:   req.SortOrder,
	}
	if err := db.DB.Create(&g).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			fail(c, http.StatusConflict, "组名已存在")
			return
		}
		fail(c, http.StatusInternalServerError, "创建失败")
		return
	}
	ok(c, g)
}

// HandleAdminUpdateGroup PUT /api/usercenter/admin/groups/:id
func HandleAdminUpdateGroup(c *gin.Context) {
	id := c.Param("id")
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	var g db.Group
	if err := db.DB.Where("id = ?", id).First(&g).Error; err != nil {
		fail(c, http.StatusNotFound, "组不存在")
		return
	}
	perms, _ := json.Marshal(req.Permissions)
	updates := map[string]interface{}{
		"name":        req.Name,
		"permissions": string(perms),
		"description": req.Description,
		"is_default":  req.IsDefault,
		"sort_order":  req.SortOrder,
	}
	if req.IsDefault {
		db.DB.Model(&db.Group{}).Where("is_default = 1 AND id != ?", id).Update("is_default", false)
	}
	if err := db.DB.Model(&db.Group{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			fail(c, http.StatusConflict, "组名已存在")
			return
		}
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	db.DB.Where("id = ?", id).First(&g)
	ok(c, g)
}

// HandleAdminDeleteGroup DELETE /api/usercenter/admin/groups/:id
func HandleAdminDeleteGroup(c *gin.Context) {
	id := c.Param("id")
	// 删除关联 user_groups
	db.DB.Where("group_id = ?", id).Delete(&db.UserGroup{})
	if err := db.DB.Where("id = ?", id).Delete(&db.Group{}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": id})
}

type setUserGroupsReq struct {
	GroupIDs  []uint     `json:"group_ids"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// HandleAdminSetUserGroups POST /api/usercenter/admin/users/:uuid/groups
// 直接覆盖用户所属组（移除原有的非 auto_assigned 组，重新设置）
func HandleAdminSetUserGroups(c *gin.Context) {
	uuid := c.Param("uuid")
	var req setUserGroupsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if _, err := db.GetUserByUUID(uuid); err != nil {
		fail(c, http.StatusNotFound, "用户不存在")
		return
	}
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// 删除原非自动分配关联
		if err := tx.Where("user_uuid = ? AND auto_assigned = 0", uuid).Delete(&db.UserGroup{}).Error; err != nil {
			return err
		}
		// 插入新关联
		for _, gid := range req.GroupIDs {
			ug := db.UserGroup{
				UserUUID:  uuid,
				GroupID:   gid,
				ExpiresAt: req.ExpiresAt,
			}
			if err := tx.Create(&ug).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "设置身份组失败")
		return
	}
	groups, _ := userGroupInfos(uuid)
	ok(c, gin.H{"groups": groups})
}

// ---------- 商品管理 ----------

// HandleAdminListProducts GET /api/usercenter/admin/products
func HandleAdminListProducts(c *gin.Context) {
	page, size := parsePage(c)
	categoryID := c.Query("category_id")
	active := c.Query("active")
	tx := db.DB.Model(&db.Product{})
	if active == "1" {
		tx = tx.Where("is_active = 1")
	} else if active == "0" {
		tx = tx.Where("is_active = 0")
	}
	if categoryID != "" {
		var cat db.Category
		if err := db.DB.Where("id = ?", categoryID).First(&cat).Error; err == nil {
			tx = tx.Where("category = ?", cat.Name)
		}
	}
	tx = tx.Order("id DESC")
	var products []db.Product
	total, err := paginate(tx.Session(&gorm.Session{}), &products, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	out := make([]gin.H, 0, len(products))
	for _, p := range products {
		out = append(out, gin.H{
			"id":           p.ID,
			"name":         p.Name,
			"description":  p.Description,
			"price":        p.Price,
			"category":     p.Category,
			"image_url":    p.ImageURL,
			"is_active":    p.IsActive,
			"sort_order":   p.SortOrder,
			"extra_config": parseExtraConfig(p.ExtraConfig),
			"stock":        p.Stock,
			"created_at":   p.CreatedAt,
			"updated_at":   p.UpdatedAt,
		})
	}
	ok(c, pageResult(out, total, page, size))
}

type productReq struct {
	Name        string         `json:"name" binding:"required"`
	Description string         `json:"description,omitempty"`
	Price       int64          `json:"price" binding:"required"`
	Category    string         `json:"category,omitempty"`
	ImageURL    string         `json:"image_url,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	SortOrder   *int           `json:"sort_order,omitempty"`
	ExtraConfig map[string]any `json:"extra_config,omitempty"`
	Stock       *int64         `json:"stock,omitempty"`
}

func (r *productReq) apply(p *db.Product, defaults bool) {
	if r.Name != "" {
		p.Name = r.Name
	}
	if r.Description != "" {
		p.Description = r.Description
	}
	if r.Price >= 0 {
		p.Price = r.Price
	}
	if r.Category != "" {
		p.Category = r.Category
	} else if defaults {
		p.Category = "默认分类"
	}
	if r.ImageURL != "" {
		p.ImageURL = r.ImageURL
	}
	if r.IsActive != nil {
		p.IsActive = *r.IsActive
	} else if defaults {
		p.IsActive = true
	}
	if r.SortOrder != nil {
		p.SortOrder = *r.SortOrder
	}
	if r.ExtraConfig != nil {
		b, _ := json.Marshal(r.ExtraConfig)
		p.ExtraConfig = string(b)
	} else if defaults {
		p.ExtraConfig = "{}"
	}
	if r.Stock != nil {
		if *r.Stock < -1 {
			*r.Stock = -1
		}
		p.Stock = *r.Stock
	} else if defaults {
		p.Stock = -1
	}
}

// HandleAdminCreateProduct POST /api/usercenter/admin/products
func HandleAdminCreateProduct(c *gin.Context) {
	var req productReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Price < 0 {
		fail(c, http.StatusBadRequest, "price 不能为负")
		return
	}
	var p db.Product
	req.apply(&p, true)
	if err := db.DB.Create(&p).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建失败")
		return
	}
	ok(c, p)
}

// HandleAdminUpdateProduct PUT /api/usercenter/admin/products/:id
func HandleAdminUpdateProduct(c *gin.Context) {
	id := c.Param("id")
	var req productReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	var p db.Product
	if err := db.DB.Where("id = ?", id).First(&p).Error; err != nil {
		fail(c, http.StatusNotFound, "商品不存在")
		return
	}
	req.apply(&p, false)
	if err := db.DB.Save(&p).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	ok(c, p)
}

// HandleAdminDeleteProduct DELETE /api/usercenter/admin/products/:id
// 软删除：is_active=false（保留历史订单外键）
func HandleAdminDeleteProduct(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Model(&db.Product{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": id, "soft_deleted": true})
}

// ---------- 分类管理 ----------

// HandleAdminListCategories GET /api/usercenter/admin/categories
func HandleAdminListCategories(c *gin.Context) {
	var cats []db.Category
	db.DB.Order("sort_order ASC, id DESC").Find(&cats)
	out := make([]gin.H, 0, len(cats))
	for _, cat := range cats {
		out = append(out, gin.H{
			"id":                cat.ID,
			"name":             cat.Name,
			"sort_order":       cat.SortOrder,
			"is_active":        cat.IsActive,
			"visible_group_ids": ParseUintArray(cat.VisibleGroupIDs),
			"created_at":       cat.CreatedAt,
		})
	}
	ok(c, out)
}

type categoryReq struct {
	Name            string `json:"name" binding:"required"`
	SortOrder       *int   `json:"sort_order,omitempty"`
	IsActive        *bool  `json:"is_active,omitempty"`
	VisibleGroupIDs []uint `json:"visible_group_ids,omitempty"`
}

func (r *categoryReq) toUpdates() map[string]interface{} {
	upd := map[string]interface{}{"name": r.Name}
	if r.SortOrder != nil {
		upd["sort_order"] = *r.SortOrder
	}
	if r.IsActive != nil {
		upd["is_active"] = *r.IsActive
	}
	if r.VisibleGroupIDs != nil {
		b, _ := json.Marshal(r.VisibleGroupIDs)
		upd["visible_group_ids"] = string(b)
	}
	return upd
}

// HandleAdminCreateCategory POST /api/usercenter/admin/categories
func HandleAdminCreateCategory(c *gin.Context) {
	var req categoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	visIDs := "[]"
	if len(req.VisibleGroupIDs) > 0 {
		b, _ := json.Marshal(req.VisibleGroupIDs)
		visIDs = string(b)
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	cat := db.Category{
		Name:            req.Name,
		SortOrder:        sortOrder,
		IsActive:        isActive,
		VisibleGroupIDs:  visIDs,
	}
	if err := db.DB.Create(&cat).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			fail(c, http.StatusConflict, "分类名已存在")
			return
		}
		fail(c, http.StatusInternalServerError, "创建失败")
		return
	}
	ok(c, cat)
}

// HandleAdminUpdateCategory PUT /api/usercenter/admin/categories/:id
func HandleAdminUpdateCategory(c *gin.Context) {
	id := c.Param("id")
	var req categoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if err := db.DB.Model(&db.Category{}).Where("id = ?", id).Updates(req.toUpdates()).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			fail(c, http.StatusConflict, "分类名已存在")
			return
		}
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	var cat db.Category
	db.DB.Where("id = ?", id).First(&cat)
	ok(c, cat)
}

// HandleAdminDeleteCategory DELETE /api/usercenter/admin/categories/:id
func HandleAdminDeleteCategory(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.Category{}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": id})
}

// ---------- 订单管理 ----------

// HandleAdminListOrders GET /api/usercenter/admin/orders
func HandleAdminListOrders(c *gin.Context) {
	page, size := parsePage(c)
	status := c.Query("status")
	userUUID := c.Query("user_uuid")
	orderNo := c.Query("order_no")
	tx := db.DB.Model(&db.Order{})
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if userUUID != "" {
		tx = tx.Where("user_uuid = ?", userUUID)
	}
	if orderNo != "" {
		tx = tx.Where("order_no LIKE ?", "%"+orderNo+"%")
	}
	tx = tx.Order("id DESC")
	var orders []db.Order
	total, err := paginate(tx.Session(&gorm.Session{}), &orders, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, pageResult(orders, total, page, size))
}

type updateOrderStatusReq struct {
	Status string `json:"status" binding:"required"`
}

// HandleAdminUpdateOrderStatus PUT /api/usercenter/admin/orders/:id/status
// status: cancelled | refunded → 退款事务（仅 paid 可退）
func HandleAdminUpdateOrderStatus(c *gin.Context) {
	id := c.Param("id")
	var req updateOrderStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status != "cancelled" && req.Status != "refunded" && req.Status != "paid" {
		fail(c, http.StatusBadRequest, "status 必须是 paid/cancelled/refunded")
		return
	}
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		var order db.Order
		if err := tx.Clauses(forUpdate()).Where("id = ?", id).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("订单不存在")
			}
			return err
		}
		// 退款：原 paid → cancelled/refunded 时退钱
		if req.Status == "cancelled" || req.Status == "refunded" {
			if order.Status != "paid" {
				return fmt.Errorf("当前订单状态不允许此操作")
			}
			if order.FinalPrice > 0 {
				wallet, err := db.EnsureUserWallet(tx, order.UserUUID)
				if err != nil {
					return err
				}
				if err := tx.Clauses(forUpdate()).Where("user_uuid = ?", order.UserUUID).First(wallet).Error; err != nil {
					return err
				}
				if err := tx.Model(&db.Wallet{}).Where("user_uuid = ?", order.UserUUID).
					UpdateColumn("balance", gorm.Expr("balance + ?", order.FinalPrice)).Error; err != nil {
					return err
				}
				oid := int64(order.ID)
				if err := tx.Create(&db.WalletTransaction{
					UserUUID:       order.UserUUID,
					Amount:         order.FinalPrice,
					Type:           "refund",
					RelatedOrderID: &oid,
					Remark:         fmt.Sprintf("订单退款: %s", order.OrderNo),
				}).Error; err != nil {
					return err
				}
			}
		}
		// 更新订单状态
		return tx.Model(&db.Order{}).Where("id = ?", id).Update("status", req.Status).Error
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "不允许") || strings.Contains(msg, "不存在") {
			fail(c, http.StatusBadRequest, msg)
			return
		}
		fail(c, http.StatusInternalServerError, "更新订单状态失败")
		return
	}
	var o db.Order
	db.DB.Where("id = ?", id).First(&o)
	ok(c, o)
}

// ---------- 兑换码管理 ----------

type generateRedeemCodesReq struct {
	Count        int    `json:"count" binding:"required,min=1,max=10000"`
	GrantType    string `json:"grant_type" binding:"required"`
	GrantAmount  int64  `json:"grant_amount" binding:"required,min=1"`
	Remark       string `json:"remark,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
}

// HandleAdminGenerateRedeemCodes POST /api/usercenter/admin/redeem-codes/generate
func HandleAdminGenerateRedeemCodes(c *gin.Context) {
	var req generateRedeemCodesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	// 校验 grant_type
	switch req.GrantType {
	case "balance", "quota", "times", "slot":
	default:
		fail(c, http.StatusBadRequest, "grant_type 必须是 balance/quota/times/slot")
		return
	}
	// 生成不重复 codes（重试避免 unique 冲突）
	codes := make([]string, 0, req.Count)
	seen := map[string]struct{}{}
	for len(codes) < req.Count {
		code := randomCode(req.Prefix, 4, 4)
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	// 批量插入
	admin := CurrentUser(c)
	records := make([]db.RedemptionCode, 0, len(codes))
	for _, code := range codes {
		records = append(records, db.RedemptionCode{
			Code:        code,
			GrantType:   req.GrantType,
			GrantAmount: req.GrantAmount,
			Remark:      req.Remark,
			// created_by 留空（admin 用户没自增 id）
		})
	}
	if err := db.DB.CreateInBatches(&records, 500).Error; err != nil {
		fail(c, http.StatusInternalServerError, "生成失败")
		return
	}
	_ = admin
	ok(c, gin.H{"codes": codes, "count": len(codes)})
}

// HandleAdminListRedeemCodes GET /api/usercenter/admin/redeem-codes
func HandleAdminListRedeemCodes(c *gin.Context) {
	page, size := parsePage(c)
	used := c.Query("used")
	code := c.Query("code")
	tx := db.DB.Model(&db.RedemptionCode{})
	if used == "1" {
		tx = tx.Where("used = 1")
	} else if used == "0" {
		tx = tx.Where("used = 0")
	}
	if code != "" {
		tx = tx.Where("code LIKE ?", "%"+strings.ToUpper(code)+"%")
	}
	tx = tx.Order("id DESC")
	var codes []db.RedemptionCode
	total, err := paginate(tx.Session(&gorm.Session{}), &codes, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, pageResult(codes, total, page, size))
}

// ---------- 流水 ----------

// HandleAdminListTransactions GET /api/usercenter/admin/transactions?type=wallet|quota|times
func HandleAdminListTransactions(c *gin.Context) {
	txType := c.DefaultQuery("type", "wallet")
	page, size := parsePage(c)
	userUUID := c.Query("user_uuid")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var total int64
	var list any
	base := db.DB
	if userUUID != "" {
		base = base.Where("user_uuid = ?", userUUID)
	}
	if startDate != "" {
		base = base.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		base = base.Where("created_at <= ?", endDate+" 23:59:59")
	}
	switch txType {
	case "wallet":
		var arr []db.WalletTransaction
		q := base.Session(&gorm.Session{}).Model(&db.WalletTransaction{}).Order("created_at DESC")
		total, _ = paginate(q, &arr, page, size)
		list = arr
	case "quota":
		var arr []db.QuotaTransaction
		q := base.Session(&gorm.Session{}).Model(&db.QuotaTransaction{}).Order("created_at DESC")
		total, _ = paginate(q, &arr, page, size)
		list = arr
	case "times":
		var arr []db.TimesTransaction
		q := base.Session(&gorm.Session{}).Model(&db.TimesTransaction{}).Order("created_at DESC")
		total, _ = paginate(q, &arr, page, size)
		list = arr
	default:
		fail(c, http.StatusBadRequest, "type 必须是 wallet/quota/times")
		return
	}
	ok(c, pageResult(list, total, page, size))
}
