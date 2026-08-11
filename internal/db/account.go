package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/Yeah114/g79client/account/mpay"
	"gorm.io/gorm"
)

// ErrNoAvailableSauth 表示 SAuth 池中没有可供自动换号使用的账号。
var ErrNoAvailableSauth = errors.New("没有可用的 SAuth")

// DeviceInfo 存储设备信息的结构体
type DeviceInfo struct {
	ID                  string `json:"id"`
	Key                 string `json:"key"`
	MCountTransactionID string `json:"mcount_transaction_id"`
	TransID             string `json:"transid"`
	UniqueID            string `json:"unique_id"`
	UDID                string `json:"udid"`
	Mac                 string `json:"mac"`
	Ram                 string `json:"ram"`
	Rom                 string `json:"rom"`
	ExtCI               string `json:"ext_ci"`
	Timestamp           int64  `json:"timestamp"`
}

// CreateUserSauthFromDeviceID 按用户当前认证状态创建并更新 Sauth。
// 已保存但尚未换取到 Sauth 的设备优先继续注册，避免验证码完成后又回到 SAuth 池。
func CreateUserSauthFromDeviceID(uuid string) error {
	user, err := GetUserByUUID(uuid)
	if err != nil {
		return fmt.Errorf("获取用户失败: %v", err)
	}

	// 如果 AutoChangeSauth=true，从 sauth 表获取数据
	if !user.IsX19 {
		return EnsureCom4399CredentialsForUser(context.Background(), uuid)
	}

	if user.Device != "" {
		return CreateUserSauthWithDeviceID(uuid)
	}

	if user.AutoChangeSauth {
		if _, err := ChangeUserSauth(uuid); err == nil {
			return nil
		}
		// SAuth 池不可用时，回退到常规注册，并保留其设备以便验证码续办。
		return CreateUserSauthWithDeviceID(uuid)
	}

	return CreateUserSauthWithDeviceID(uuid)
}

// CreateUserSauthWithDeviceID 使用用户已保存的设备或新设备执行常规游客注册。
// 新设备会先保存，即使本次需要验证码失败，也可在下次请求继续使用该设备。
func CreateUserSauthWithDeviceID(uuid string) error {
	user, err := GetUserByUUID(uuid)
	if err != nil {
		return fmt.Errorf("获取用户失败: %v", err)
	}

	var device *mpay.Device

	if user.Device != "" {
		// 如果数据库中已有设备信息，解析JSON获取设备信息
		var deviceInfo DeviceInfo
		if err := json.Unmarshal([]byte(user.Device), &deviceInfo); err != nil {
			return fmt.Errorf("解析设备信息失败: %v", err)
		}

		device = &mpay.Device{
			ID:                  deviceInfo.ID,
			Key:                 deviceInfo.Key,
			MCountTransactionID: deviceInfo.MCountTransactionID,
			TransID:             deviceInfo.TransID,
			UniqueID:            deviceInfo.UniqueID,
			UDID:                deviceInfo.UDID,
			Mac:                 deviceInfo.Mac,
			Ram:                 deviceInfo.Ram,
			Rom:                 deviceInfo.Rom,
			ExtCI:               deviceInfo.ExtCI,
			Timestamp:           deviceInfo.Timestamp,
		}
	} else {
		// 如果数据库中没有设备信息，生成新设备并保存到数据库
		newDevice, err := mpay.GenerateDevice(context.Background())
		if err != nil {
			return fmt.Errorf("生成设备失败: %v", err)
		}

		// 构建设备信息结构体
		deviceInfo := DeviceInfo{
			ID:                  newDevice.ID,
			Key:                 newDevice.Key,
			MCountTransactionID: newDevice.MCountTransactionID,
			TransID:             newDevice.TransID,
			UniqueID:            newDevice.UniqueID,
			UDID:                newDevice.UDID,
			Mac:                 newDevice.Mac,
			Ram:                 "8034369536",   // defaultRAMBytes
			Rom:                 "128849018880", // defaultROMBytes
			ExtCI:               "",
			Timestamp:           time.Now().Unix(),
		}

		// 序列化设备信息为JSON并保存到数据库
		deviceJSON, err := json.Marshal(deviceInfo)
		if err != nil {
			return fmt.Errorf("序列化设备信息失败: %v", err)
		}

		if err := UpdateUserDevice(uuid, string(deviceJSON)); err != nil {
			return fmt.Errorf("保存设备信息到数据库失败: %v", err)
		}

		device = newDevice
	}

	// 尝试获取游客账号
	guest, err := device.Guest(context.Background())
	if err != nil {
		var verifyErr *mpay.NeedVerifyError
		if errors.As(err, &verifyErr) {
			if verifyErr.Code == 1351 {
				msg, fetchErr := fetchAndExtractMessageFromURL(verifyErr.VerifyURL)
				if fetchErr != nil {
					return fmt.Errorf("%s (code=%d), %s", verifyErr.Reason, verifyErr.Code, verifyErr.VerifyURL)
				}
				return fmt.Errorf("%s (code=%d), %s", verifyErr.Reason, verifyErr.Code, msg)
			}
			return fmt.Errorf("%s (code=%d), %s", verifyErr.Reason, verifyErr.Code, verifyErr.VerifyURL)
		}
		return fmt.Errorf("生成游客账号失败: %v", err)
	}

	// 额外注册4个游客账号到SauthPool表。该功能基于网易漏洞，验证完成一次后短时间内无需再次验证，可能会随时失效。
	for i := 0; i < 4; i++ {
		extraGuest, err := device.Guest(context.Background())
		if err != nil {
			var verifyErr *mpay.NeedVerifyError
			if errors.As(err, &verifyErr) {
				log.Printf("额外第 %d 个游客需要验证，跳过: %s (code=%d)\n", i+1, verifyErr.Reason, verifyErr.Code)
			} else {
				log.Printf("额外第 %d 个游客生成失败: %v\n", i+1, err)
			}
			break
		}
		extraSauthJSON, _ := json.Marshal(extraGuest.Sauth)
		escapedExtraSauthJSON, _ := json.Marshal(string(extraSauthJSON))
		extraCookiePayload := fmt.Sprintf(`{"sauth_json":%s}`, string(escapedExtraSauthJSON))
		sauthPool := &SauthPool{Sauth: extraCookiePayload}
		if err := DB.Create(sauthPool).Error; err != nil {
			log.Printf("保存额外第 %d 个游客账号失败: %v\n", i+1, err)
		} else {
			log.Printf("额外第 %d 个游客账号已保存到 SAuth 池 (id=%d)\n", i+1, sauthPool.ID)
		}
	}

	// 将Sauth信息序列化为字符串
	sauthJSON, err := json.Marshal(guest.Sauth)
	if err != nil {
		return fmt.Errorf("序列化Sauth失败: %v", err)
	}

	// 正确转义嵌入的JSON字符串
	escapedSauthJSON, err := json.Marshal(string(sauthJSON))
	if err != nil {
		return fmt.Errorf("转义Sauth JSON失败: %v", err)
	}

	// 创建cookie字符串
	cookiePayload := fmt.Sprintf(`{"sauth_json":%s}`,
		string(escapedSauthJSON))

	// 更新数据库中的Sauth字段并清空设备信息
	if err := UpdateUserSauth(uuid, cookiePayload); err != nil {
		return fmt.Errorf("更新用户Sauth失败: %v", err)
	}

	return nil
}

// UpdateUserDevice 更新用户的设备信息
func UpdateUserDevice(uuid, device string) error {
	updates := map[string]interface{}{
		"device": device,
	}
	result := DB.Model(&User{}).Where("uuid = ?", uuid).Updates(updates)
	return result.Error
}

// UpdateUserSauth 更新用户的Sauth并保留Device信息
func UpdateUserSauth(uuid, sauth string) error {
	// 先获取当前用户信息，以保留设备信息
	user, err := GetUserByUUID(uuid)
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}

	updates := map[string]interface{}{
		"sauth":  sauth,
		"device": user.Device,
	}
	result := DB.Model(&User{}).Where("uuid = ?", uuid).Updates(updates)
	return result.Error
}

// UpdateUserSauthOnly 仅更新用户的Sauth，不修改Device
func UpdateUserSauthOnly(uuid, sauth string) error {
	result := DB.Model(&User{}).Where("uuid = ?", uuid).Update("sauth", sauth)
	return result.Error
}

// UpdateUserDeviceAndSauthToNull 清空用户的Device和Sauth信息
func UpdateUserDeviceAndSauthToNull(uuid string) error {
	updates := map[string]interface{}{
		"sauth":  "",
		"device": "",
	}
	result := DB.Model(&User{}).Where("uuid = ?", uuid).Updates(updates)
	return result.Error
}

// UpdateUserDeviceFullInfo 更新用户的完整设备信息（兼容旧方法，现在存储为JSON）
func UpdateUserDeviceFullInfo(uuid, deviceID, deviceKey, mcountTransactionID, transID, uniqueID, udid, mac string) error {
	// 构建设备信息结构体
	deviceInfo := DeviceInfo{
		ID:                  deviceID,
		Key:                 deviceKey,
		MCountTransactionID: mcountTransactionID,
		TransID:             transID,
		UniqueID:            uniqueID,
		UDID:                udid,
		Mac:                 mac,
		Ram:                 "8034369536",
		Rom:                 "128849018880",
		ExtCI:               "",
		Timestamp:           time.Now().Unix(),
	}

	// 序列化设备信息为JSON
	deviceJSON, err := json.Marshal(deviceInfo)
	if err != nil {
		return fmt.Errorf("序列化设备信息失败: %v", err)
	}

	return UpdateUserDevice(uuid, string(deviceJSON))
}

// UpdateUserDeviceID 更新用户的DeviceID（兼容旧方法，现在存储为JSON）
func UpdateUserDeviceID(uuid, deviceID string) error {
	user, err := GetUserByUUID(uuid)
	if err != nil {
		return err
	}

	var deviceInfo DeviceInfo
	if user.Device != "" {
		// 如果已有设备信息，解析并更新ID
		if err := json.Unmarshal([]byte(user.Device), &deviceInfo); err != nil {
			// 如果解析失败，创建新的设备信息
			deviceInfo = DeviceInfo{
				Ram:       "8034369536",
				Rom:       "128849018880",
				Timestamp: time.Now().Unix(),
			}
		}
	}

	deviceInfo.ID = deviceID

	deviceJSON, err := json.Marshal(deviceInfo)
	if err != nil {
		return err
	}

	return UpdateUserDevice(uuid, string(deviceJSON))
}

// GetRandomAvailableSauth 从 SAuth 池中随机获取一个可用的 SAuth（Bantime 为空）
func GetRandomAvailableSauth() (*SauthPool, error) {
	var sauth SauthPool
	result := DB.Where("bantime IS NULL").Order("RAND()").First(&sauth)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNoAvailableSauth
		}
		return nil, result.Error
	}
	return &sauth, nil
}

// ChangeUserSauth 清空用户当前的认证信息，并分配一个可用的 SAuth。
// 清空 device 是为了避免本次换号继续沿用上一个失效账号的设备信息。
func ChangeUserSauth(userUUID string) (*SauthPool, error) {
	if err := UpdateUserDeviceAndSauthToNull(userUUID); err != nil {
		return nil, fmt.Errorf("清空用户 SAuth 和设备信息失败: %w", err)
	}

	sauthPool, err := GetRandomAvailableSauth()
	if err != nil {
		return nil, err
	}
	if err := AssignSauthToUser(userUUID, sauthPool.ID); err != nil {
		return nil, fmt.Errorf("分配 SAuth 失败: %w", err)
	}
	return sauthPool, nil
}

// MarkSauthAsBanned 标记 SAuth 为已封禁
func MarkSauthAsBanned(id uint) error {
	now := time.Now()
	return DB.Model(&SauthPool{}).Where("Id = ?", id).Update("bantime", now).Error
}

// AssignSauthToUser 将 SAuth 池中的 SAuth 分配给用户
func AssignSauthToUser(userUUID string, sauthPoolID uint) error {
	var sauthPool SauthPool
	if err := DB.First(&sauthPool, sauthPoolID).Error; err != nil {
		return err
	}

	result := DB.Model(&User{}).Where("uuid = ?", userUUID).Update("sauth", sauthPool.Sauth)
	return result.Error
}

// fetchAndExtractMessageFromURL 请求URL并从HTML中提取消息
func fetchAndExtractMessageFromURL(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	html := string(htmlBytes)
	return extractMessageFromHTML(html)
}

// extractMessageFromHTML 从HTML中提取消息
func extractMessageFromHTML(html string) (string, error) {
	msgPattern := regexp.MustCompile(`<p id="msg">(.*?)</p>`)
	matches := msgPattern.FindStringSubmatch(html)
	if len(matches) < 2 {
		return "", fmt.Errorf("未找到消息")
	}
	fullMsg := matches[1]

	extractPattern := regexp.MustCompile(`(账号存在安全风险.*?进行验证)`)
	extractMatches := extractPattern.FindStringSubmatch(fullMsg)
	if len(extractMatches) < 1 {
		return fullMsg, nil
	}
	return extractMatches[0], nil
}
