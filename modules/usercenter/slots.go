package usercenter

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Yeah114/FunAuth/internal/db"
)

// HandleMySlots GET /api/usercenter/slots/mine
func HandleMySlots(c *gin.Context) {
	user := CurrentUser(c)
	var slots []db.Slot
	// 注：slots 表当前 schema 没有 created_at，按 end_time 排序（NULL 在后）
	db.DB.Where("user_uuid = ?", user.UUID).Order("end_time DESC").Find(&slots)
	if slots == nil {
		slots = []db.Slot{}
	}
	ok(c, gin.H{"list": slots, "total": len(slots)})
}

type bindSlotReq struct {
	ServerCode string `json:"server_code" binding:"required"`
}

// HandleBindSlot POST /api/usercenter/slots/:id/bind
func HandleBindSlot(c *gin.Context) {
	user := CurrentUser(c)
	id := c.Param("id")
	var req bindSlotReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if req.ServerCode == "" {
		fail(c, http.StatusBadRequest, "server_code 不能为空")
		return
	}
	var slot db.Slot
	err := db.DB.Where("id = ? AND user_uuid = ?", id, user.UUID).First(&slot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "卡槽不存在或不属于当前用户")
			return
		}
		fail(c, http.StatusInternalServerError, "查询卡槽失败")
		return
	}
	if err := db.DB.Model(&db.Slot{}).Where("id = ?", slot.ID).Updates(map[string]interface{}{
		"server_code": req.ServerCode,
	}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "绑定失败")
		return
	}
	db.DB.Where("id = ?", slot.ID).First(&slot)
	ok(c, slot)
}

// HandleUnbindSlot POST /api/usercenter/slots/:id/unbind
func HandleUnbindSlot(c *gin.Context) {
	user := CurrentUser(c)
	id := c.Param("id")
	var slot db.Slot
	err := db.DB.Where("id = ? AND user_uuid = ?", id, user.UUID).First(&slot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "卡槽不存在或不属于当前用户")
			return
		}
		fail(c, http.StatusInternalServerError, "查询卡槽失败")
		return
	}
	if err := db.DB.Model(&db.Slot{}).Where("id = ?", slot.ID).Update("server_code", "").Error; err != nil {
		fail(c, http.StatusInternalServerError, "解绑失败")
		return
	}
	db.DB.Where("id = ?", slot.ID).First(&slot)
	ok(c, slot)
}

// ---- Admin Slots ----

// HandleAdminListSlots GET /api/usercenter/admin/slots
func HandleAdminListSlots(c *gin.Context) {
	page, size := parsePage(c)
	userUUID := c.Query("user_uuid")
	serverCode := c.Query("server_code")
	tx := db.DB.Model(&db.Slot{})
	if userUUID != "" {
		tx = tx.Where("user_uuid = ?", userUUID)
	}
	if serverCode != "" {
		tx = tx.Where("server_code = ?", serverCode)
	}
	var slots []db.Slot
	total, err := paginate(tx.Session(&gorm.Session{}).Order("id DESC"), &slots, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, pageResult(slots, total, page, size))
}

type grantSlotsReq struct {
	UserUUID     string `json:"user_uuid" binding:"required"`
	Count        int    `json:"count" binding:"required,min=1,max=1000"`
	DurationDays *int   `json:"duration_days,omitempty"`
}

// HandleAdminGrantSlots POST /api/usercenter/admin/slots/grant
func HandleAdminGrantSlots(c *gin.Context) {
	var req grantSlotsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	// 校验用户存在
	if existed, _ := db.GetUserByUUID(req.UserUUID); existed == nil {
		fail(c, http.StatusNotFound, "用户不存在")
		return
	}
	var endTime *time.Time
	if req.DurationDays != nil && *req.DurationDays > 0 {
		t := time.Now().Add(time.Duration(*req.DurationDays) * 24 * time.Hour)
		endTime = &t
	}
	slots := make([]db.Slot, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		slots = append(slots, db.Slot{
			ID:         uuid.New().String(),
			UserUUID:   req.UserUUID,
			ServerCode: "",
			EndTime:    endTime,
		})
	}
	if err := db.DB.CreateInBatches(&slots, 200).Error; err != nil {
		fail(c, http.StatusInternalServerError, "发放卡槽失败")
		return
	}
	ok(c, gin.H{"granted": len(slots), "end_time": endTime})
}

// HandleAdminDeleteSlot DELETE /api/usercenter/admin/slots/:id
func HandleAdminDeleteSlot(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.Slot{}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": id})
}
