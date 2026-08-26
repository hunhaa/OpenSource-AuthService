package usercenter

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Yeah114/FunAuth/internal/db"
)

// HandleListAnnouncements GET /api/usercenter/announcements
func HandleListAnnouncements(c *gin.Context) {
	page, size := parsePage(c)
	tx := db.DB.Model(&db.Announcement{}).Order("date DESC, id DESC")
	var items []db.Announcement
	total, err := paginate(tx.Session(&gorm.Session{}), &items, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询公告失败")
		return
	}
	ok(c, pageResult(items, total, page, size))
}

// ---- Admin ----

// HandleAdminListAnnouncements GET /api/usercenter/admin/announcements
func HandleAdminListAnnouncements(c *gin.Context) {
	page, size := parsePage(c)
	tx := db.DB.Model(&db.Announcement{}).Order("id DESC")
	var items []db.Announcement
	total, err := paginate(tx.Session(&gorm.Session{}), &items, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询公告失败")
		return
	}
	ok(c, pageResult(items, total, page, size))
}

type announcementReq struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content" binding:"required"`
	Author  string `json:"author,omitempty"`
}

// HandleAdminCreateAnnouncement POST /api/usercenter/admin/announcements
func HandleAdminCreateAnnouncement(c *gin.Context) {
	var req announcementReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	user := CurrentUser(c)
	author := req.Author
	if author == "" {
		author = user.Username
	}
	ann := db.Announcement{
		Title:   req.Title,
		Content: req.Content,
		Author:  author,
		Date:    time.Now(),
	}
	if err := db.DB.Create(&ann).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建公告失败")
		return
	}
	ok(c, ann)
}

// HandleAdminUpdateAnnouncement PUT /api/usercenter/admin/announcements/:id
func HandleAdminUpdateAnnouncement(c *gin.Context) {
	id := c.Param("id")
	var req announcementReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	var ann db.Announcement
	if err := db.DB.Where("id = ?", id).First(&ann).Error; err != nil {
		fail(c, http.StatusNotFound, "公告不存在")
		return
	}
	updates := map[string]interface{}{
		"title":   req.Title,
		"content": req.Content,
	}
	if req.Author != "" {
		updates["author"] = req.Author
	}
	if err := db.DB.Model(&db.Announcement{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新公告失败")
		return
	}
	db.DB.Where("id = ?", id).First(&ann)
	ok(c, ann)
}

// HandleAdminDeleteAnnouncement DELETE /api/usercenter/admin/announcements/:id
func HandleAdminDeleteAnnouncement(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.Announcement{}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": id})
}
