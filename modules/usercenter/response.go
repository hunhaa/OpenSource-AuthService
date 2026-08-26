package usercenter

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// R 统一响应格式
type R struct {
	OK   bool   `json:"ok"`
	Msg  string `json:"msg,omitempty"`
	Data any    `json:"data,omitempty"`
	Code int    `json:"code,omitempty"`
}

// PageResult 分页响应数据
type PageResult struct {
	List       any   `json:"list"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
	TotalPages int   `json:"total_pages"`
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, R{OK: true, Data: data})
}

func okMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, R{OK: true, Msg: msg})
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, R{OK: false, Msg: msg, Code: code})
}

func failMsg(c *gin.Context, code int, format string, args ...any) {
	c.JSON(code, R{OK: false, Msg: fmt.Sprintf(format, args...), Code: code})
}

// pageResult 构造分页数据
func pageResult(list any, total int64, page, size int) PageResult {
	if size <= 0 {
		size = 20
	}
	tp := int((total + int64(size) - 1) / int64(size))
	if tp < 0 {
		tp = 0
	}
	return PageResult{List: list, Total: total, Page: page, Size: size, TotalPages: tp}
}

// parsePage 解析分页参数（page 默认1，size 默认20 最大100）
func parsePage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

// offset 计算 SQL OFFSET
func offset(page, size int) int {
	if page <= 0 {
		page = 1
	}
	return (page - 1) * size
}

// paginate 对 *gorm.DB 查询应用 Limit/Offset 并返回总数 + 数据
func paginate(tx *gorm.DB, dst any, page, size int) (int64, error) {
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return 0, err
	}
	if err := tx.Limit(size).Offset(offset(page, size)).Find(dst).Error; err != nil {
		return 0, err
	}
	return total, nil
}
