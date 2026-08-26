package usercenter

import (
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

type redeemReq struct {
	Code string `json:"code" binding:"required"`
}

// HandleRedeem POST /api/usercenter/redeem
// 幂等：UPDATE redemption_codes SET used=1, used_by=?, used_at=? WHERE code=? AND used=0，RowsAffected=1 才成功
func HandleRedeem(c *gin.Context) {
	user := CurrentUser(c)
	var req redeemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		fail(c, http.StatusBadRequest, "兑换码不能为空")
		return
	}

	var granted gin.H
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// 1) 幂等占用兑换码
		res := tx.Model(&db.RedemptionCode{}).
			Where("code = ? AND used = 0", code).
			Updates(map[string]interface{}{
				"used":    true,
				"used_by": user.UUID,
				"used_at": time.Now(),
				"updated_at": time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 检查是不是不存在
			var rc db.RedemptionCode
			if err := tx.Where("code = ?", code).First(&rc).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("兑换码不存在")
				}
				return err
			}
			return fmt.Errorf("兑换码已被使用")
		}
		// 2) 读取兑换码发放配置
		var rc db.RedemptionCode
		if err := tx.Where("code = ?", code).First(&rc).Error; err != nil {
			return err
		}
		amount := rc.GrantAmount
		if amount <= 0 {
			return fmt.Errorf("兑换码配置无效")
		}
		// 3) 按发放类型执行业务逻辑
		switch rc.GrantType {
		case "balance":
			// 锁行 + 加余额 + 写流水
			wallet, err := db.EnsureUserWallet(tx, user.UUID)
			if err != nil {
				return err
			}
			if err := tx.Model(&db.Wallet{}).Where("user_uuid = ?", user.UUID).
				UpdateColumn("balance", gorm.Expr("balance + ?", amount)).Error; err != nil {
				return err
			}
			wallet.Balance += amount
			if err := tx.Create(&db.WalletTransaction{
				UserUUID: user.UUID,
				Amount:   amount,
				Type:     "redeem_grant",
				Remark:   "兑换码兑现余额",
			}).Error; err != nil {
				return err
			}
			granted = gin.H{"balance_added": amount}
		case "quota":
			if err := tx.Model(&db.User{}).Where("uuid = ?", user.UUID).
				UpdateColumn("quota", gorm.Expr("quota + ?", amount)).Error; err != nil {
				return err
			}
			if err := tx.Create(&db.QuotaTransaction{
				UserUUID: user.UUID,
				Amount:   amount,
				Type:     "redeem_grant",
				Remark:   "兑换码兑现额度",
			}).Error; err != nil {
				return err
			}
			granted = gin.H{"quota_added": amount}
		case "times":
			if err := tx.Model(&db.User{}).Where("uuid = ?", user.UUID).
				UpdateColumn("times", gorm.Expr("times + ?", amount)).Error; err != nil {
				return err
			}
			if err := tx.Create(&db.TimesTransaction{
				UserUUID: user.UUID,
				Amount:   amount,
				Type:     "redeem_grant",
				Remark:   "兑换码兑现次数",
			}).Error; err != nil {
				return err
			}
			granted = gin.H{"times_added": amount}
		case "slot":
			// 批量创建卡槽（amount = 卡槽数量）
			if amount <= 0 {
				return fmt.Errorf("兑换码配置的卡槽数量无效")
			}
			slots := make([]db.Slot, 0, amount)
			for i := int64(0); i < amount; i++ {
				slots = append(slots, db.Slot{
					ID:         newUUID(),
					UserUUID:   user.UUID,
					ServerCode: "",
				})
			}
			if err := tx.CreateInBatches(&slots, 200).Error; err != nil {
				return err
			}
			granted = gin.H{"slots_added": amount}
		default:
			return fmt.Errorf("不支持的兑换码类型: %s", rc.GrantType)
		}
		return nil
	})
	if err != nil {
		// 区分已知业务错误与服务器错误
		msg := err.Error()
		if strings.Contains(msg, "已被使用") || strings.Contains(msg, "不存在") || strings.Contains(msg, "无效") || strings.Contains(msg, "不支持") {
			fail(c, http.StatusBadRequest, msg)
			return
		}
		fail(c, http.StatusInternalServerError, "兑换失败")
		return
	}
	ok(c, gin.H{"message": "兑换成功", "code": code, "granted": granted})
}

// newUUID 包装 uuid.New 便于测试
func newUUID() string {
	return uuid.NewString()
}
