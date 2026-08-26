package db

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ---------- 用户中心新增表模型 ----------

// Group 身份组
type Group struct {
	ID          uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(128);not null;uniqueIndex" json:"name"`
	Permissions string    `gorm:"column:permissions;type:text;not null;default:'[]'" json:"permissions"`
	Description string    `gorm:"column:description;type:varchar(255)" json:"description"`
	IsDefault   bool      `gorm:"column:is_default;type:tinyint(1);not null;default:0" json:"is_default"`
	SortOrder   int       `gorm:"column:sort_order;type:int;not null;default:0" json:"sort_order"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Group) TableName() string { return "groups" }

// UserGroup 用户-组关联
type UserGroup struct {
	ID           uint       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserUUID     string     `gorm:"column:user_uuid;type:varchar(36);not null;index" json:"user_uuid"`
	GroupID      uint       `gorm:"column:group_id;not null" json:"group_id"`
	ExpiresAt    *time.Time `gorm:"column:expires_at;type:datetime" json:"expires_at"`
	AutoAssigned bool       `gorm:"column:auto_assigned;type:tinyint(1);not null;default:0" json:"auto_assigned"`
	CreatedAt    time.Time  `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (UserGroup) TableName() string { return "user_groups" }

// Wallet 钱包
type Wallet struct {
	ID        uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserUUID  string    `gorm:"column:user_uuid;type:varchar(36);not null;uniqueIndex" json:"user_uuid"`
	Balance   int64     `gorm:"column:balance;type:bigint;not null;default:0" json:"balance"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Wallet) TableName() string { return "wallets" }

// WalletTransaction 钱包流水
type WalletTransaction struct {
	ID             uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserUUID       string    `gorm:"column:user_uuid;type:varchar(36);not null;index" json:"user_uuid"`
	Amount         int64     `gorm:"column:amount;type:bigint;not null" json:"amount"`
	Type           string    `gorm:"column:type;type:varchar(32);not null;default:'payment'" json:"type"`
	RelatedOrderID *int64    `gorm:"column:related_order_id;type:bigint" json:"related_order_id"`
	Remark         string    `gorm:"column:remark;type:varchar(255)" json:"remark"`
	CreatedAt      time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (WalletTransaction) TableName() string { return "wallet_transactions" }

// Product 商品
type Product struct {
	ID          uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Description string    `gorm:"column:description;type:text" json:"description"`
	Price       int64     `gorm:"column:price;type:bigint;not null" json:"price"`
	Category    string    `gorm:"column:category;type:varchar(128);not null;default:'默认分类'" json:"category"`
	ImageURL    string    `gorm:"column:image_url;type:varchar(512)" json:"image_url"`
	IsActive    bool      `gorm:"column:is_active;type:tinyint(1);not null;default:1" json:"is_active"`
	SortOrder   int       `gorm:"column:sort_order;type:int;not null;default:0" json:"sort_order"`
	ExtraConfig string    `gorm:"column:extra_config;type:text;not null;default:'{}'" json:"extra_config"`
	Stock       int64     `gorm:"column:stock;type:bigint;not null;default:-1" json:"stock"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Product) TableName() string { return "products" }

// Category 商品分类
type Category struct {
	ID              uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name            string    `gorm:"column:name;type:varchar(128);not null;uniqueIndex" json:"name"`
	SortOrder       int       `gorm:"column:sort_order;type:int;not null;default:0" json:"sort_order"`
	IsActive        bool      `gorm:"column:is_active;type:tinyint(1);not null;default:1" json:"is_active"`
	VisibleGroupIDs string    `gorm:"column:visible_group_ids;type:text;not null;default:'[]'" json:"visible_group_ids"`
	CreatedAt       time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Category) TableName() string { return "categories" }

// Order 订单
type Order struct {
	ID             uint       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	OrderNo        string     `gorm:"column:order_no;type:varchar(64);not null;uniqueIndex" json:"order_no"`
	UserUUID       string     `gorm:"column:user_uuid;type:varchar(36);not null;index" json:"user_uuid"`
	ProductID      *uint      `gorm:"column:product_id" json:"product_id"`
	Quantity       int        `gorm:"column:quantity;type:int;not null;default:1" json:"quantity"`
	OriginalPrice  int64      `gorm:"column:original_price;type:bigint;not null" json:"original_price"`
	DiscountAmount int64      `gorm:"column:discount_amount;type:bigint;not null;default:0" json:"discount_amount"`
	FinalPrice     int64      `gorm:"column:final_price;type:bigint;not null" json:"final_price"`
	DiscountInfo   string     `gorm:"column:discount_info;type:text" json:"discount_info"`
	Status         string     `gorm:"column:status;type:varchar(16);not null;default:'pending'" json:"status"`
	PaymentMethod  string     `gorm:"column:payment_method;type:varchar(64)" json:"payment_method"`
	CreatedAt      time.Time  `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Order) TableName() string { return "orders" }

// RedemptionCode 兑换码
type RedemptionCode struct {
	ID          uint       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Code        string     `gorm:"column:code;type:varchar(64);not null;uniqueIndex" json:"code"`
	GrantType   string     `gorm:"column:grant_type;type:varchar(16);not null" json:"grant_type"`
	GrantAmount int64      `gorm:"column:grant_amount;type:bigint;not null;default:0" json:"grant_amount"`
	Used        bool       `gorm:"column:used;type:tinyint(1);not null;default:0" json:"used"`
	UsedBy      string     `gorm:"column:used_by;type:varchar(64)" json:"used_by"`
	UsedAt      *time.Time `gorm:"column:used_at;type:datetime" json:"used_at"`
	Remark      string     `gorm:"column:remark;type:varchar(255)" json:"remark"`
	CreatedBy   *uint      `gorm:"column:created_by;type:bigint" json:"created_by"`
	CreatedAt   time.Time  `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (RedemptionCode) TableName() string { return "redemption_codes" }

// APIKey API密钥
type APIKey struct {
	ID         uint       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserUUID   string     `gorm:"column:user_uuid;type:varchar(36);not null;index" json:"user_uuid"`
	KeyHash    string     `gorm:"column:key_hash;type:varchar(64);not null;index" json:"key_hash"`
	KeyPlain   string     `gorm:"column:key_plain;type:varchar(128);not null" json:"key_plain"`
	Prefix     string     `gorm:"column:prefix;type:varchar(16);not null" json:"prefix"`
	Remark     string     `gorm:"column:remark;type:varchar(255)" json:"remark"`
	Revoked    bool       `gorm:"column:revoked;type:tinyint(1);not null;default:0" json:"revoked"`
	CreatedAt  time.Time  `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	LastUsedAt *time.Time `gorm:"column:last_used_at;type:datetime" json:"last_used_at"`
	ExpiresAt  *time.Time `gorm:"column:expires_at;type:datetime" json:"expires_at"`
	UsageCount int64      `gorm:"column:usage_count;type:bigint;not null;default:0" json:"usage_count"`
}

func (APIKey) TableName() string { return "api_keys" }

// Announcement 公告
type Announcement struct {
	ID        uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Title     string    `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Author    string    `gorm:"column:author;type:varchar(128);not null;default:'admin'" json:"author"`
	Date      time.Time `gorm:"column:date;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"date"`
	Content   string    `gorm:"column:content;type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Announcement) TableName() string { return "announcements" }

// SystemSetting 系统设置（单例 id=1）
type SystemSetting struct {
	ID                          uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	RateLimitPerMin             int64     `gorm:"column:rate_limit_per_min;type:bigint;not null;default:0" json:"rate_limit_per_min"`
	IPWhitelist                 string    `gorm:"column:ip_whitelist;type:text" json:"ip_whitelist"`
	IPBlacklist                 string    `gorm:"column:ip_blacklist;type:text" json:"ip_blacklist"`
	AutoBlacklistEnabled        bool      `gorm:"column:auto_blacklist_enabled;type:tinyint(1);not null;default:0" json:"auto_blacklist_enabled"`
	AutoBlacklistThreshold      int       `gorm:"column:auto_blacklist_threshold;type:int;not null;default:50" json:"auto_blacklist_threshold"`
	AutoBlacklistDurationMinutes int      `gorm:"column:auto_blacklist_duration_minutes;type:int;not null;default:60" json:"auto_blacklist_duration_minutes"`
	LoginEnabled                bool      `gorm:"column:login_enabled;type:tinyint(1);not null;default:1" json:"login_enabled"`
	RegisterEnabled             bool      `gorm:"column:register_enabled;type:tinyint(1);not null;default:1" json:"register_enabled"`
	LoginAllowedGroupIDs        string    `gorm:"column:login_allowed_group_ids;type:text" json:"login_allowed_group_ids"`
	MaintenanceEnabled          bool      `gorm:"column:maintenance_enabled;type:tinyint(1);not null;default:0" json:"maintenance_enabled"`
	MaintenanceWhitelistIPs     string    `gorm:"column:maintenance_whitelist_ips;type:text" json:"maintenance_whitelist_ips"`
	MaintenanceMessage          string    `gorm:"column:maintenance_message;type:varchar(500)" json:"maintenance_message"`
	ExtraConfig                 string    `gorm:"column:extra_config;type:text" json:"extra_config"`
	UpdatedAt                   time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// QuotaTransaction 额度流水
type QuotaTransaction struct {
	ID             uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserUUID       string    `gorm:"column:user_uuid;type:varchar(36);not null;index" json:"user_uuid"`
	Amount         int64     `gorm:"column:amount;type:bigint;not null" json:"amount"`
	Type           string    `gorm:"column:type;type:varchar(32);not null;default:'adjust'" json:"type"`
	RelatedOrderID *int64    `gorm:"column:related_order_id;type:bigint" json:"related_order_id"`
	Remark         string    `gorm:"column:remark;type:varchar(255)" json:"remark"`
	CreatedAt      time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (QuotaTransaction) TableName() string { return "quota_transactions" }

// TimesTransaction 次数流水
type TimesTransaction struct {
	ID             uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserUUID       string    `gorm:"column:user_uuid;type:varchar(36);not null;index" json:"user_uuid"`
	Amount         int64     `gorm:"column:amount;type:bigint;not null" json:"amount"`
	Type           string    `gorm:"column:type;type:varchar(32);not null;default:'adjust'" json:"type"`
	RelatedOrderID *int64    `gorm:"column:related_order_id;type:bigint" json:"related_order_id"`
	Remark         string    `gorm:"column:remark;type:varchar(255)" json:"remark"`
	CreatedAt      time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (TimesTransaction) TableName() string { return "times_transactions" }

// autoMigrateUserCenterTables 创建用户中心所需的全部表（仅 CREATE TABLE IF NOT EXISTS，已存在表保持原样）
func autoMigrateUserCenterTables() error {
	if DB == nil {
		return fmt.Errorf("DB not initialized")
	}
	models := []interface{}{
		&Group{},
		&UserGroup{},
		&Wallet{},
		&WalletTransaction{},
		&Product{},
		&Category{},
		&Order{},
		&RedemptionCode{},
		&APIKey{},
		&Announcement{},
		&SystemSetting{},
		&QuotaTransaction{},
		&TimesTransaction{},
	}
	for _, m := range models {
		// AutoMigrate 只会创建缺失的表/列，已存在表不会被破坏
		if err := DB.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate %T: %w", m, err)
		}
	}
	// 系统设置单例：不存在则插入默认
	ensureDefaultSystemSetting()
	return nil
}

// ensureDefaultSystemSetting 确保系统设置单例 id=1 存在
func ensureDefaultSystemSetting() {
	var count int64
	DB.Model(&SystemSetting{}).Where("id = 1").Count(&count)
	if count == 0 {
		DB.Create(&SystemSetting{
			ID:             1,
			LoginEnabled:   true,
			RegisterEnabled: true,
		})
	}
}

// EnsureUserWallet 确保用户钱包存在，不存在则创建（balance=0）
func EnsureUserWallet(tx *gorm.DB, userUUID string) (*Wallet, error) {
	var w Wallet
	err := tx.Where("user_uuid = ?", userUUID).First(&w).Error
	if err == nil {
		return &w, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	w = Wallet{UserUUID: userUUID, Balance: 0}
	if err := tx.Create(&w).Error; err != nil {
		return nil, err
	}
	return &w, nil
}
