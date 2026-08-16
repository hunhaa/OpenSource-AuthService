package db

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// maskDSNPassword 把 user:pass@tcp(...) 中的 pass 替换为 ***，避免错误日志泄露密码
func maskDSNPassword(dsn string) string {
	d := strings.TrimSpace(dsn)
	at := strings.Index(d, "@")
	if at < 0 {
		return d
	}
	up := d[:at]
	colon := strings.Index(up, ":")
	if colon < 0 {
		return d
	}
	user := up[:colon]
	return user + ":***@" + d[at+1:]
}

type User struct {
	UUID            string     `gorm:"primaryKey;column:uuid;type:varchar(36);not null" json:"uuid"`
	Username        string     `gorm:"column:username;type:varchar(50);not null;index" json:"username"`
	Password        string     `gorm:"column:password;type:varchar(255);not null" json:"password"`
	Token           string     `gorm:"column:token;type:varchar(255);not null" json:"token"`
	Sauth           string     `gorm:"column:sauth;type:text" json:"sauth"`
	Device          string     `gorm:"column:device;type:text" json:"device"`
	AutoChangeSauth bool       `gorm:"column:auto_change_sauth;type:tinyint(1);default:false" json:"auto_change_sauth"`
	IsX19           bool       `gorm:"column:isX19;type:tinyint(1);not null;default:true" json:"isX19"`
	NicknamePrefix  string     `gorm:"column:nickname_prefix;type:varchar(50);default:'HZ'" json:"nickname_prefix"`
	EndTime         *time.Time `gorm:"column:end_time;type:datetime" json:"end_time"`
}

func (User) TableName() string {
	return "users"
}

type Slot struct {
	ID         string     `gorm:"primaryKey;column:id;type:varchar(36);not null" json:"id"`
	UserUUID   string     `gorm:"column:user_uuid;type:varchar(36);not null;index" json:"user_uuid"`
	ServerCode string     `gorm:"column:server_code;type:varchar(50);not null" json:"server_code"`
	EndTime    *time.Time `gorm:"column:end_time;type:datetime" json:"end_time"`
}

func (Slot) TableName() string {
	return "slots"
}

type IDCode struct {
	ID     int    `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name   string `gorm:"column:name;type:varchar(255);not null" json:"name"`
	IDCard string `gorm:"column:id_card;type:varchar(255);not null" json:"id_card"`
}

func (IDCode) TableName() string {
	return "idcode"
}

type SauthPool struct {
	ID      uint       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Sauth   string     `gorm:"column:sauth;type:text" json:"sauth"`
	Bantime *time.Time `gorm:"column:bantime;type:datetime" json:"bantime"`
}

type Account struct {
	ID       uint   `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Type     string `gorm:"column:type;type:varchar(32);not null;index" json:"type"`
	Username string `gorm:"column:username;type:varchar(64);not null;uniqueIndex" json:"username"`
	Password string `gorm:"column:password;type:varchar(255);not null" json:"password"`
	Sauth    string `gorm:"column:sauth;type:text" json:"sauth"`
}

func (Account) TableName() string { return "account" }

type InitOptions struct {
	StartCom4399AccountPool bool
}

func (SauthPool) TableName() string {
	return "sauth"
}

func InitDB() error {
	return InitDBWithOptions(InitOptions{StartCom4399AccountPool: true})
}

func InitDBWithOptions(options InitOptions) error {
	cfg, _ := LoadConfig()
	dsn := ResolveDSN(cfg)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect database (dsn=%q): %w", maskDSNPassword(dsn), err)
	}

	DB = db
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("get sql database handle: %w", err)
	}
	// Keep database concurrency bounded so registration and account-write
	// workers can make progress without creating unbounded MySQL connections.
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	if err := DB.AutoMigrate(&Account{}); err != nil {
		return fmt.Errorf("migrate account table: %w", err)
	}

	//表已经全部存在无需迁移
	//DB.AutoMigrate(...)

	if err := fixSchemaIssues(); err != nil {
		return fmt.Errorf("failed to fix schema issues: %v", err)
	}
	initIDCodeCache()
	initAccountWriteQueue()

	go startCleanupTask()

	// 异步初始化 4399 账号缓存池
	if options.StartCom4399AccountPool {
		go InitCom4399AccountPool()
	}

	return nil
}

func fixSchemaIssues() error {
	fmt.Println("[DB] Checking and fixing table structure...")

	if err := fixSlotsTable(); err != nil {
		return err
	}

	if err := fixUsersTable(); err != nil {
		return err
	}

	if err := fixSauthTable(); err != nil {
		return err
	}

	if err := fixIdcodeTable(); err != nil {
		return err
	}

	fmt.Println("[DB] Table structure check completed")
	return nil
}

func fixUsersTable() error {
	var tableExists bool
	DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'users'").Scan(&tableExists)

	if !tableExists {
		return nil
	}

	var exactColumnExists bool
	DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'isX19'").Scan(&exactColumnExists)
	if exactColumnExists {
		return nil
	}

	var snakeColumnExists bool
	DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'is_x19'").Scan(&snakeColumnExists)
	if snakeColumnExists {
		fmt.Println("[DB] Detected users.is_x19 column, renaming to isX19...")
		if err := DB.Exec("ALTER TABLE users CHANGE COLUMN is_x19 isX19 tinyint(1) NOT NULL DEFAULT 1").Error; err != nil {
			return fmt.Errorf("failed to rename users.is_x19 column to isX19: %v", err)
		}
		fmt.Println("[DB] Renamed users.is_x19 column to isX19")
		return nil
	}

	fmt.Println("[DB] Detected missing users.isX19 column, adding...")
	if err := DB.Exec("ALTER TABLE users ADD COLUMN isX19 tinyint(1) NOT NULL DEFAULT 1 AFTER auto_change_sauth").Error; err != nil {
		return fmt.Errorf("failed to add users.isX19 column: %v", err)
	}

	fmt.Println("[DB] Added users.isX19 column")
	return nil
}

func fixSlotsTable() error {
	var tableExists bool
	DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'slots'").Scan(&tableExists)

	if !tableExists {
		return nil
	}

	var columnInfo struct {
		Extra string
	}
	result := DB.Raw("SELECT EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'slots' AND COLUMN_NAME = 'id'").Scan(&columnInfo)

	if result.Error != nil || result.RowsAffected == 0 {
		return nil
	}

	if containsAutoIncrement(columnInfo.Extra) {
		fmt.Println("[DB] Detected invalid auto_increment on slots.id, fixing...")

		if err := DB.Exec("ALTER TABLE slots MODIFY COLUMN id varchar(36) NOT NULL").Error; err != nil {
			return fmt.Errorf("failed to remove auto_increment from slots.id: %v", err)
		}

		fmt.Println("[DB] Fixed slots table structure")
	}

	return nil
}

func fixSauthTable() error {
	var tableExists bool
	DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'sauth'").Scan(&tableExists)

	if !tableExists {
		return nil
	}

	type ColumnInfo struct {
		ColumnName string
		DataType   string
		Extra      string
	}

	var columns []ColumnInfo
	DB.Raw("SELECT COLUMN_NAME, DATA_TYPE, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sauth' ORDER BY ORDINAL_POSITION").Scan(&columns)

	hasLowercaseId := false

	for _, col := range columns {
		switch col.ColumnName {
		case "id":
			hasLowercaseId = true

		case "Id", "ID":
			fmt.Println("[DB] Detected uppercase Id column in sauth table, fixing...")

			if hasLowercaseId {
				fmt.Println("[DB] Lowercase id already exists, dropping duplicate Id column...")
				if err := DB.Exec(fmt.Sprintf("ALTER TABLE sauth DROP COLUMN `%s`", col.ColumnName)).Error; err != nil {
					return fmt.Errorf("failed to drop duplicate sauth.%s column: %v", col.ColumnName, err)
				}
			} else {
				var extra string
				DB.Raw(fmt.Sprintf("SELECT EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sauth' AND COLUMN_NAME = '%s'", col.ColumnName)).Scan(&extra)

				autoInc := ""
				if containsAutoIncrement(extra) {
					autoInc = " AUTO_INCREMENT"
				}

				if err := DB.Exec(fmt.Sprintf("ALTER TABLE sauth CHANGE COLUMN `%s` id int unsigned%s NOT NULL", col.ColumnName, autoInc)).Error; err != nil {
					return fmt.Errorf("failed to rename sauth.%s column: %v", col.ColumnName, err)
				}
			}

			fmt.Println("[DB] Fixed sauth table Id column")

		case "Sauth":
			fmt.Println("[DB] Detected uppercase Sauth column in sauth table, fixing...")

			if err := DB.Exec("ALTER TABLE sauth CHANGE COLUMN Sauth sauth TEXT").Error; err != nil {
				return fmt.Errorf("failed to rename sauth.Sauth column: %v", err)
			}

			fmt.Println("[DB] Fixed sauth table Sauth column")

		case "Bantime":
			fmt.Println("[DB] Detected uppercase Bantime column in sauth table, fixing...")

			if err := DB.Exec("ALTER TABLE sauth CHANGE COLUMN Bantime bantime DATETIME NULL").Error; err != nil {
				return fmt.Errorf("failed to rename sauth.Bantime column: %v", err)
			}

			fmt.Println("[DB] Fixed sauth table Bantime column")
		}
	}

	return nil
}

func fixIdcodeTable() error {
	var tableExists bool
	DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'idcode'").Scan(&tableExists)

	if !tableExists {
		return nil
	}

	type ColumnInfo struct {
		ColumnName string
		Extra      string
	}

	var columns []ColumnInfo
	DB.Raw("SELECT COLUMN_NAME, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'idcode'").Scan(&columns)

	for _, col := range columns {
		if col.ColumnName == "id" && !containsAutoIncrement(col.Extra) {
			fmt.Println("[DB] Detected missing auto_increment on idcode.id, adding...")

			if err := DB.Exec("ALTER TABLE idcode MODIFY COLUMN id int NOT NULL AUTO_INCREMENT").Error; err != nil {
				return fmt.Errorf("failed to add auto_increment to idcode.id: %v", err)
			}

			fmt.Println("[DB] Fixed idcode table structure")
		}
	}

	return nil
}

func containsAutoIncrement(extra string) bool {
	return len(extra) >= 14 && (extra[:14] == "auto_increment" ||
		(len(extra) > 14 && extra[:15] == "auto_increment "))
}
