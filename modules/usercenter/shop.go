package usercenter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Yeah114/FunAuth/internal/db"
)

// ---- 商品/分类（公开） ----

// HandleShopCategories GET /api/usercenter/shop/categories
func HandleShopCategories(c *gin.Context) {
	user := CurrentUser(c)
	var myGroupIDs []uint
	if user != nil {
		myGroupIDs, _ = UserGroupIDs(user.UUID)
	}
	var cats []db.Category
	db.DB.Where("is_active = 1").Order("sort_order ASC, id DESC").Find(&cats)
	out := make([]gin.H, 0, len(cats))
	for _, cat := range cats {
		visIDs := ParseUintArray(cat.VisibleGroupIDs)
		if len(visIDs) > 0 && !uintSliceIntersect(visIDs, myGroupIDs) {
			continue
		}
		out = append(out, gin.H{
			"id":                cat.ID,
			"name":             cat.Name,
			"sort_order":       cat.SortOrder,
			"is_active":        cat.IsActive,
			"visible_group_ids": visIDs,
		})
	}
	ok(c, out)
}

// HandleShopProducts GET /api/usercenter/shop/products?category_id=
func HandleShopProducts(c *gin.Context) {
	user := CurrentUser(c)
	page, size := parsePage(c)
	categoryID := c.Query("category_id")
	myGroupIDs, _ := UserGroupIDs(user.UUID)

	tx := db.DB.Model(&db.Product{}).Where("is_active = 1")
	// 按分类过滤（分类 id → name）
	if categoryID != "" {
		var cat db.Category
		if err := db.DB.Where("id = ?", categoryID).First(&cat).Error; err == nil {
			tx = tx.Where("category = ?", cat.Name)
		}
	}
	tx = tx.Order("sort_order ASC, id DESC")
	var products []db.Product
	total, err := paginate(tx.Session(&gorm.Session{}), &products, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询商品失败")
		return
	}
	// 过滤可见性（extra_config.visible_group_ids）
	filtered := make([]gin.H, 0, len(products))
	for _, p := range products {
		cfg := parseExtraConfig(p.ExtraConfig)
		if visIDs, ok := cfg["visible_group_ids"]; ok {
			if arr, ok2 := visIDs.([]any); ok2 && len(arr) > 0 {
				ids := toUintSlice(arr)
				if len(ids) > 0 && !uintSliceIntersect(ids, myGroupIDs) {
					continue
				}
			}
		}
		filtered = append(filtered, gin.H{
			"id":           p.ID,
			"name":         p.Name,
			"description":  p.Description,
			"price":        p.Price,
			"category":     p.Category,
			"image_url":    p.ImageURL,
			"sort_order":   p.SortOrder,
			"stock":        p.Stock,
			"extra_config": cfg,
		})
	}
	ok(c, pageResult(filtered, total, page, size))
}

type createOrderReq struct {
	ProductID uint `json:"product_id" binding:"required"`
	Quantity int  `json:"quantity" binding:"required,min=1"`
}

// HandleCreateOrder POST /api/usercenter/shop/orders  创建订单 + 余额支付（单事务）
func HandleCreateOrder(c *gin.Context) {
	user := CurrentUser(c)
	var req createOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	qty := req.Quantity
	if qty < 1 {
		qty = 1
	}

	var orderNo string
	var resp gin.H
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// 1) 锁定商品行
		var product db.Product
		if err := tx.Clauses(forUpdate()).
			Where("id = ?", req.ProductID).First(&product).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("商品不存在")
			}
			return err
		}
		if !product.IsActive {
			return fmt.Errorf("商品已下架")
		}
		// 2) 库存校验
		if product.Stock >= 0 && int64(qty) > product.Stock {
			return fmt.Errorf("库存不足")
		}
		// 3) 可见性校验
		cfg := parseExtraConfig(product.ExtraConfig)
		if visIDs, ok := cfg["visible_group_ids"]; ok {
			if arr, ok2 := visIDs.([]any); ok2 && len(arr) > 0 {
				ids := toUintSlice(arr)
				if len(ids) > 0 {
					myIDs, _ := UserGroupIDs(user.UUID)
					if !uintSliceIntersect(ids, myIDs) {
						return fmt.Errorf("无权限购买该商品")
					}
				}
			}
		}
		// 4) 价格计算（暂不支持折扣）
		original := product.Price * int64(qty)
		discount := int64(0)
		finalPrice := original - discount

		// 5) 锁定钱包行 + 余额校验
		wallet, err := db.EnsureUserWallet(tx, user.UUID)
		if err != nil {
			return err
		}
		if err := tx.Clauses(forUpdate()).Where("user_uuid = ?", user.UUID).First(wallet).Error; err != nil {
			return err
		}
		if finalPrice > 0 && wallet.Balance < finalPrice {
			return fmt.Errorf("余额不足")
		}

		// 6) 创建订单（status=paid）
		orderNo = fmt.Sprintf("%s%s", time.Now().Format("20060102150405"), strings.ToUpper(uuid.NewString()[:8]))
		discountInfo, _ := json.Marshal(gin.H{})
		order := db.Order{
			OrderNo:        orderNo,
			UserUUID:       user.UUID,
			ProductID:      &product.ID,
			Quantity:        qty,
			OriginalPrice:  original,
			DiscountAmount: discount,
			FinalPrice:     finalPrice,
			DiscountInfo:   string(discountInfo),
			Status:         "paid",
			PaymentMethod:  "balance",
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		// 7) 扣余额 + 写流水
		if finalPrice > 0 {
			if err := tx.Model(&db.Wallet{}).Where("user_uuid = ?", user.UUID).
				UpdateColumn("balance", gorm.Expr("balance - ?", finalPrice)).Error; err != nil {
				return err
			}
			orderID := int64(order.ID)
			if err := tx.Create(&db.WalletTransaction{
				UserUUID:       user.UUID,
				Amount:         -finalPrice,
				Type:           "shop_consume",
				RelatedOrderID: &orderID,
				Remark:         fmt.Sprintf("购买商品: %s x%d", product.Name, qty),
			}).Error; err != nil {
				return err
			}
		}

		// 8) 扣库存
		if product.Stock >= 0 {
			if err := tx.Model(&db.Product{}).Where("id = ?", product.ID).
				UpdateColumn("stock", gorm.Expr("stock - ?", qty)).Error; err != nil {
				return err
			}
		}

		// 9) 发货（按 extra_config.grant_type）
		if err := settleProductGrant(tx, user.UUID, order.ID, product, cfg, qty); err != nil {
			return err
		}

		resp = gin.H{
			"order_id":   order.ID,
			"order_no":  orderNo,
			"status":    order.Status,
			"final_price": finalPrice,
			"message":    "支付成功",
		}
		return nil
	})
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "商品不存在"), strings.Contains(msg, "商品已下架"),
			strings.Contains(msg, "库存不足"), strings.Contains(msg, "余额不足"),
			strings.Contains(msg, "无权限"):
			fail(c, http.StatusBadRequest, msg)
		default:
			fail(c, http.StatusInternalServerError, "下单失败")
		}
		return
	}
	ok(c, resp)
}

// HandleMyOrders GET /api/usercenter/shop/orders/mine
func HandleMyOrders(c *gin.Context) {
	user := CurrentUser(c)
	page, size := parsePage(c)
	status := c.Query("status")
	tx := db.DB.Model(&db.Order{}).Where("user_uuid = ?", user.UUID)
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	tx = tx.Order("id DESC")
	var orders []db.Order
	total, err := paginate(tx.Session(&gorm.Session{}), &orders, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询订单失败")
		return
	}
	// 附带商品名
	out := make([]gin.H, 0, len(orders))
	pids := map[uint]struct{}{}
	for _, o := range orders {
		if o.ProductID != nil {
			pids[*o.ProductID] = struct{}{}
		}
	}
	prods := map[uint]db.Product{}
	if len(pids) > 0 {
		ids := make([]uint, 0, len(pids))
		for id := range pids {
			ids = append(ids, id)
		}
		var ps []db.Product
		db.DB.Where("id IN ?", ids).Find(&ps)
		for _, p := range ps {
			prods[p.ID] = p
		}
	}
	for _, o := range orders {
		item := gin.H{
			"id":              o.ID,
			"order_no":        o.OrderNo,
			"product_id":      o.ProductID,
			"quantity":        o.Quantity,
			"original_price":  o.OriginalPrice,
			"discount_amount": o.DiscountAmount,
			"final_price":     o.FinalPrice,
			"status":          o.Status,
			"payment_method":  o.PaymentMethod,
			"created_at":      o.CreatedAt,
		}
		if o.ProductID != nil {
			if p, has := prods[*o.ProductID]; has {
				item["product_name"] = p.Name
			}
		}
		out = append(out, item)
	}
	ok(c, pageResult(out, total, page, size))
}

// ---- 发货逻辑 ----

// settleProductGrant 按商品 extra_config.grant_type 发货
func settleProductGrant(tx *gorm.DB, userUUID string, orderID uint, product db.Product, cfg map[string]any, qty int) error {
	gt, _ := cfg["grant_type"].(string)
	switch gt {
	case "balance":
		amount := toInt64(cfg["grant_value"]) * int64(qty)
		if amount <= 0 {
			return fmt.Errorf("商品配置 grant_value 无效")
		}
		if err := tx.Model(&db.Wallet{}).Where("user_uuid = ?", userUUID).
			UpdateColumn("balance", gorm.Expr("balance + ?", amount)).Error; err != nil {
			return err
		}
		oid := int64(orderID)
		return tx.Create(&db.WalletTransaction{
			UserUUID:       userUUID,
			Amount:         amount,
			Type:           "shop_grant",
			RelatedOrderID: &oid,
			Remark:         fmt.Sprintf("购买商品发放余额: %s", product.Name),
		}).Error
	case "quota":
		amount := toInt64(cfg["grant_value"]) * int64(qty)
		if amount <= 0 {
			return fmt.Errorf("商品配置 grant_value 无效")
		}
		if err := tx.Model(&db.User{}).Where("uuid = ?", userUUID).
			UpdateColumn("quota", gorm.Expr("quota + ?", amount)).Error; err != nil {
			return err
		}
		oid := int64(orderID)
		return tx.Create(&db.QuotaTransaction{
			UserUUID:       userUUID,
			Amount:         amount,
			Type:           "shop_grant",
			RelatedOrderID: &oid,
			Remark:         fmt.Sprintf("购买商品发放额度: %s", product.Name),
		}).Error
	case "times":
		amount := toInt64(cfg["grant_value"]) * int64(qty)
		if amount <= 0 {
			return fmt.Errorf("商品配置 grant_value 无效")
		}
		if err := tx.Model(&db.User{}).Where("uuid = ?", userUUID).
			UpdateColumn("times", gorm.Expr("times + ?", amount)).Error; err != nil {
			return err
		}
		oid := int64(orderID)
		return tx.Create(&db.TimesTransaction{
			UserUUID:       userUUID,
			Amount:         amount,
			Type:           "shop_grant",
			RelatedOrderID: &oid,
			Remark:         fmt.Sprintf("购买商品发放次数: %s", product.Name),
		}).Error
	case "slot":
		count := int(toInt64(cfg["grant_value"])) * qty
		if count <= 0 {
			return fmt.Errorf("商品配置 grant_value 无效")
		}
		slots := make([]db.Slot, 0, count)
		for i := 0; i < count; i++ {
			slots = append(slots, db.Slot{
				ID:         uuid.NewString(),
				UserUUID:   userUUID,
				ServerCode: "",
			})
		}
		return tx.CreateInBatches(&slots, 200).Error
	case "", "none":
		// 无发货逻辑（普通虚拟商品）
		return nil
	default:
		return fmt.Errorf("不支持的发货类型: %s", gt)
	}
}

// ---- 辅助 ----

func parseExtraConfig(raw string) map[string]any {
	cfg := map[string]any{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return map[string]any{}
	}
	return cfg
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
	}
	return 0
}

func toUintSlice(arr []any) []uint {
	out := make([]uint, 0, len(arr))
	for _, v := range arr {
		switch x := v.(type) {
		case float64:
			if x > 0 {
				out = append(out, uint(x))
			}
		case int:
			if x > 0 {
				out = append(out, uint(x))
			}
		case string:
			// 尝试解析
			var n uint
			if _, err := fmt.Sscanf(x, "%d", &n); err == nil && n > 0 {
				out = append(out, n)
			}
		}
	}
	return out
}

func uintSliceIntersect(a, b []uint) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := map[uint]struct{}{}
	for _, x := range b {
		set[x] = struct{}{}
	}
	for _, x := range a {
		if _, ok := set[x]; ok {
			return true
		}
	}
	return false
}
