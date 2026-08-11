package db

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GenerateUUID 生成UUID，如果输入为空则自动生成
func GenerateUUID(input string) string {
	if input != "" {
		return input
	}
	return uuid.New().String()
}

// GenerateToken 根据用户名和密码生成token
func GenerateToken(username, password string) (string, error) {

	if username == "" || password == "" {
		return "", fmt.Errorf("username or password cannot be empty")
	}

	data := username + ":YeahYYDSYuansiAuthService:" + password
	hasher := sha256.New()
	hasher.Write([]byte(data))
	hashBytes := hasher.Sum(nil)
	base64Str := base64.StdEncoding.EncodeToString(hashBytes)

	return "hz/" + base64Str, nil

}

// CreateUser 创建用户
func CreateUser(user *User) error {
	// 如果UUID为空，自动生成
	user.UUID = GenerateUUID(user.UUID)

	// 如果token为空，根据用户名和原始密码生成
	if user.Token == "" {
		token, err := GenerateToken(user.Username, user.Password)
		if err != nil {
			return fmt.Errorf("failed to generate token: %v", err)
		}
		user.Token = token
	}

	// 对密码进行SHA-256哈希处理（在生成token之后）
	passwordHasher := sha256.New()
	passwordHasher.Write([]byte(user.Password))
	user.Password = hex.EncodeToString(passwordHasher.Sum(nil))

	// 确保设备信息字段初始化为空字符串
	if user.Device == "" {
		user.Device = ""
	}

	return DB.Create(user).Error
}

// CreateAccount persists a batch-created external account without applying the
// user-table password hashing and token behavior.
func CreateAccount(account *Account) error {
	if account == nil {
		return errors.New("account is nil")
	}
	return DB.Create(account).Error
}

func createAccounts(accounts []*Account) error {
	if len(accounts) == 0 {
		return nil
	}
	return DB.CreateInBatches(accounts, len(accounts)).Error
}

const (
	accountWriteBatchSize  = 500
	accountWriteInterval   = 10 * time.Second
	accountWriteFlushDelay = accountWriteInterval
	accountWriteMaxWorkers = 4
	accountWriteTimeout    = 30 * time.Second
)

var accountWriteQueue struct {
	sync.Mutex
	pending  []*Account
	inFlight int
	signal   chan struct{}
	writers  chan struct{}
	once     sync.Once
}

func initAccountWriteQueue() {
	accountWriteQueue.once.Do(func() {
		accountWriteQueue.signal = make(chan struct{}, 1)
		accountWriteQueue.writers = make(chan struct{}, accountWriteMaxWorkers)
		go accountWriteTickerWorker()
	})
}

// QueueAccount submits an account for asynchronous persistence and returns
// without waiting for the single database connection.
func QueueAccount(account *Account) error {
	if account == nil {
		return errors.New("account is nil")
	}
	initAccountWriteQueue()
	copy := *account
	accountWriteQueue.Lock()
	accountWriteQueue.pending = append(accountWriteQueue.pending, &copy)
	accountWriteQueue.Unlock()
	return nil
}

func accountWriteWorker() {
	for range accountWriteQueue.signal {
		for {
			accountWriteQueue.Lock()
			if len(accountWriteQueue.pending) == 0 {
				accountWriteQueue.Unlock()
				break
			}
			account := accountWriteQueue.pending[0]
			accountWriteQueue.pending = accountWriteQueue.pending[1:]
			accountWriteQueue.inFlight++
			accountWriteQueue.Unlock()

			if err := CreateAccount(account); err != nil {
				log.Printf("账号异步入库失败: username=%s err=%v", account.Username, err)
			}
			accountWriteQueue.Lock()
			accountWriteQueue.inFlight--
			accountWriteQueue.Unlock()
		}
	}
}

func accountWriteTickerWorker() {
	ticker := time.NewTicker(accountWriteInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			launchAccountWriteBatches()
		case <-accountWriteQueue.signal:
			launchAccountWriteBatches()
		}
	}
}

func launchAccountWriteBatches() {
	for {
		batch := takeAccountWriteBatch(accountWriteBatchSize)
		if len(batch) == 0 {
			return
		}
		go persistAccountWriteBatch(batch)
	}
}

func persistAccountWriteBatch(batch []*Account) {
	accountWriteQueue.writers <- struct{}{}
	defer func() {
		<-accountWriteQueue.writers
		finishAccountWriteBatch(len(batch))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), accountWriteTimeout)
	defer cancel()
	if err := createAccountsWithContext(ctx, batch); err != nil {
		if isRetryableAccountWriteError(err) || ctx.Err() != nil {
			log.Printf("账号批量入库失败，已放回缓存重试: count=%d err=%v", len(batch), err)
			requeueAccountWriteBatch(batch)
			return
		}
		log.Printf("账号批量入库失败: count=%d err=%v", len(batch), err)
		if isRetryableAccountWriteError(err) || ctx.Err() != nil {
			log.Printf("账号批量入库失败，已放回缓存重试: count=%d err=%v", len(batch), err)
			requeueAccountWriteBatch(batch)
			return
		}
		for _, account := range batch {
			itemCtx, itemCancel := context.WithTimeout(context.Background(), accountWriteTimeout)
			if err := DB.WithContext(itemCtx).Create(account).Error; err != nil {
				if isRetryableAccountWriteError(err) || itemCtx.Err() != nil {
					log.Printf("账号单条入库失败，已放回缓存重试: username=%s err=%v", account.Username, err)
					requeueAccountWriteBatch([]*Account{account})
				} else {
					log.Printf("账号异步入库失败: username=%s err=%v", account.Username, err)
				}
			}
			itemCancel()
		}
		return
	}
	log.Printf("账号批量入库成功: count=%d", len(batch))
}

func createAccountsWithContext(ctx context.Context, accounts []*Account) error {
	if len(accounts) == 0 {
		return nil
	}
	return DB.WithContext(ctx).CreateInBatches(accounts, len(accounts)).Error
}

func isRetryableAccountWriteError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "duplicate") || strings.Contains(message, "1062") {
		return false
	}
	return strings.Contains(message, "timeout") ||
		strings.Contains(message, "deadline exceeded") ||
		strings.Contains(message, "too many connections") ||
		strings.Contains(message, "bad connection") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "deadlock") ||
		strings.Contains(message, "lock wait")
}

func accountWriteBatchWorker() {
	for range accountWriteQueue.signal {
		for {
			time.Sleep(accountWriteFlushDelay)
			batch := takeAccountWriteBatch(accountWriteBatchSize)
			if len(batch) == 0 {
				break
			}

			if err := createAccounts(batch); err != nil {
				log.Printf("账号批量入库失败: count=%d err=%v", len(batch), err)
				for _, account := range batch {
					if err := CreateAccount(account); err != nil {
						log.Printf("账号异步入库失败: username=%s err=%v", account.Username, err)
					}
				}
			} else {
				log.Printf("账号批量入库成功: count=%d", len(batch))
			}
			finishAccountWriteBatch(len(batch))
		}
	}
}

func takeAccountWriteBatch(limit int) []*Account {
	accountWriteQueue.Lock()
	defer accountWriteQueue.Unlock()
	if len(accountWriteQueue.pending) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(accountWriteQueue.pending) {
		limit = len(accountWriteQueue.pending)
	}
	batch := make([]*Account, limit)
	copy(batch, accountWriteQueue.pending[:limit])
	accountWriteQueue.pending = accountWriteQueue.pending[limit:]
	accountWriteQueue.inFlight += len(batch)
	return batch
}

func finishAccountWriteBatch(count int) {
	accountWriteQueue.Lock()
	accountWriteQueue.inFlight -= count
	if accountWriteQueue.inFlight < 0 {
		accountWriteQueue.inFlight = 0
	}
	accountWriteQueue.Unlock()
}

func requeueAccountWriteBatch(batch []*Account) {
	if len(batch) == 0 {
		return
	}
	accountWriteQueue.Lock()
	accountWriteQueue.pending = append(batch, accountWriteQueue.pending...)
	accountWriteQueue.Unlock()
}

func AccountWriteStats() (pending int, inFlight int) {
	accountWriteQueue.Lock()
	defer accountWriteQueue.Unlock()
	return len(accountWriteQueue.pending), accountWriteQueue.inFlight
}

func WaitAccountWrites(ctx context.Context) error {
	for {
		accountWriteQueue.Lock()
		pending := len(accountWriteQueue.pending)
		empty := pending == 0 && accountWriteQueue.inFlight == 0
		accountWriteQueue.Unlock()
		if empty {
			return nil
		}
		if pending > 0 {
			select {
			case accountWriteQueue.signal <- struct{}{}:
			default:
			}
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// GetUserByUUID 根据UUID获取用户
func GetUserByUUID(uuid string) (*User, error) {
	var user User
	result := DB.Where("uuid = ?", uuid).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

// GetUserByUsername 根据用户名获取用户
func GetUserByUsername(username string) (*User, error) {
	var user User
	result := DB.Where("username = ?", username).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

// GetUserByToken 根据token获取用户
func GetUserByToken(token string) (*User, error) {
	var user User
	result := DB.Where("token = ?", token).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

// ValidateFBToken 验证FBToken是否有效且在有效期内
func ValidateFBToken(token string) (*User, error) {
	user, err := GetUserByToken(token)
	if err != nil {
		return nil, fmt.Errorf("FBToken不存在: %v", err)
	}

	// 检查用户是否在有效期内
	if user.EndTime != nil && user.EndTime.Before(time.Now()) {
		return nil, fmt.Errorf("用户已过期")
	}

	return user, nil
}

// ValidateUserCredentials 验证用户名和密码是否正确且在有效期内
func ValidateUserCredentials(username, password string) (*User, error) {
	user, err := GetUserByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("用户不存在: %v", err)
	}
	if user.Password != password {
		return nil, fmt.Errorf("密码错误")
	}

	// 检查用户是否在有效期内
	if user.EndTime != nil && user.EndTime.Before(time.Now()) {
		return nil, fmt.Errorf("用户已过期")
	}

	return user, nil
}

// UpdateUser 更新用户信息
func UpdateUser(uuid string, updates map[string]interface{}) error {
	result := DB.Model(&User{}).Where("uuid = ?", uuid).Updates(updates)
	return result.Error
}

// CreateSlot 创建卡槽
func CreateSlot(slot *Slot) error {
	if slot.ID == "" {
		slot.ID = uuid.New().String()
	}
	return DB.Create(slot).Error
}

// GetSlotByUserUUIDAndServerCode 根据用户UUID和服务器代码获取卡槽
func GetSlotByUserUUIDAndServerCode(userUUID, serverCode string) (*Slot, error) {
	var slot Slot
	result := DB.Where("user_uuid = ? AND server_code = ?", userUUID, serverCode).First(&slot)
	if result.Error != nil {
		return nil, result.Error
	}
	return &slot, nil
}

// GetSlotsByUserUUID 根据用户UUID获取所有卡槽
func GetSlotsByUserUUID(userUUID string) ([]*Slot, error) {
	var slots []*Slot
	result := DB.Where("user_uuid = ?", userUUID).Find(&slots)
	if result.Error != nil {
		return nil, result.Error
	}
	return slots, nil
}

// HasSlotForServer 检查用户是否有指定服务器的卡槽权限
func HasSlotForServer(username, serverCode string) (bool, error) {

	// 首先获取用户信息
	user, err := GetUserByUsername(username)
	if err != nil {
		return false, err
	}

	// 检查是否有ALL卡槽（允许进入所有服务器）
	allSlot := &Slot{}
	result := DB.Where("user_uuid = ? AND server_code = ?", user.UUID, "ALL").First(allSlot)
	if result.Error == nil {
		// 如果ALL卡槽存在且未过期，则允许进入
		if allSlot.EndTime == nil || allSlot.EndTime.After(time.Now()) {
			return true, nil
		}
	} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, result.Error
	}

	// 检查是否有指定服务器的卡槽
	slot := &Slot{}
	result = DB.Where("user_uuid = ? AND server_code = ?", user.UUID, serverCode).First(slot)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 没有找到指定服务器的卡槽，返回false
			return false, nil
		}
		return false, result.Error
	}

	// 检查卡槽是否过期
	if slot.EndTime != nil && slot.EndTime.Before(time.Now()) {
		return false, nil
	}

	return true, nil
}

// UpdateSlot 更新卡槽信息
func UpdateSlot(id string, updates map[string]interface{}) error {
	result := DB.Model(&Slot{}).Where("id = ?", id).Updates(updates)
	return result.Error
}

// DeleteSlot 删除卡槽
func DeleteSlot(id string) error {
	result := DB.Delete(&Slot{}, "id = ?", id)
	return result.Error
}

// DeleteExpiredSlots 删除过期卡槽
func DeleteExpiredSlots() error {
	now := time.Now()
	result := DB.Where("end_time IS NOT NULL AND end_time < ?", now).Delete(&Slot{})
	return result.Error
}

// startCleanupTask 启动定时清理任务
func startCleanupTask() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	cleanup()

	for range ticker.C {
		cleanup()
	}
}

func cleanup() {
	// 删除过期的Slot
	now := time.Now()
	result := DB.Where("end_time IS NOT NULL AND end_time < ?", now).Delete(&Slot{})
	if result.Error != nil {
		log.Printf("清理过期Slot失败: %v", result.Error)
	}

	// 为没有token的用户生成token
	var users []*User
	userResult := DB.Where("token = '' OR token IS NULL").Find(&users)
	if userResult.Error != nil {
		log.Printf("查询无token用户失败: %v", userResult.Error)
	} else {
		updatedCount := 0
		for _, user := range users {
			// 生成新的token
			token, err := GenerateToken(user.Username, user.Password)
			if err != nil {
				userIdentifier := user.Username
				if userIdentifier == "" {
					userIdentifier = user.UUID
				}
				// 如果是因为用户名或密码为空，删除该用户
				if strings.Contains(err.Error(), "username or password cannot be empty") {
					if deleteErr := DB.Delete(&User{}, "uuid = ?", user.UUID).Error; deleteErr != nil {
						log.Printf("删除用户 %s 失败: %v", userIdentifier, deleteErr)
					}
				}
				continue
			}

			// 更新用户token
			updateResult := DB.Model(&User{}).Where("uuid = ?", user.UUID).Update("token", token)
			if updateResult.Error != nil {
				log.Printf("更新用户 %s 的token失败: %v", user.Username, updateResult.Error)
			} else {
				updatedCount++
			}
		}
	}

	// 可以在这里添加清理过期用户的逻辑（如果需要）
	// 目前需求是用户到达EndTime不做处理，所以暂时不实现
}

// IsAdultIDCard 判断身份证号对应的人是否已成年（>=18岁）
func IsAdultIDCard(idCard string) bool {
	idCard = strings.TrimSpace(idCard)
	if len(idCard) != 18 {
		return false
	}

	birthStr := idCard[6:14]
	birthDate, err := time.Parse("20060102", birthStr)
	if err != nil {
		return false
	}

	now := time.Now()
	age := now.Year() - birthDate.Year()
	if now.YearDay() < birthDate.YearDay() {
		age--
	}

	return age >= 18
}

// GetRandomIDCode 随机获取一个已成年的身份证信息
const idCodeCacheSize = 10

var idCodeCache struct {
	sync.Mutex
	records []*IDCode
	filling bool
	target  int
}

func initIDCodeCache() {
	idCodeCache.Lock()
	idCodeCache.records = nil
	idCodeCache.filling = false
	idCodeCache.target = idCodeCacheSize
	idCodeCache.Unlock()
	go refillIDCodeCache()
}

func SetIDCodeCacheTarget(target int) {
	if target < idCodeCacheSize {
		target = idCodeCacheSize
	}
	idCodeCache.Lock()
	idCodeCache.target = target
	needMore := len(idCodeCache.records) < target
	idCodeCache.Unlock()
	if needMore {
		go refillIDCodeCache()
	}
}

func IDCodeCacheStats() (cached int, target int) {
	idCodeCache.Lock()
	defer idCodeCache.Unlock()
	target = idCodeCache.target
	if target < idCodeCacheSize {
		target = idCodeCacheSize
	}
	return len(idCodeCache.records), target
}

func refillIDCodeCache() {
	idCodeCache.Lock()
	target := idCodeCache.target
	if target < idCodeCacheSize {
		target = idCodeCacheSize
	}
	if idCodeCache.filling || len(idCodeCache.records) >= target {
		idCodeCache.Unlock()
		return
	}
	idCodeCache.filling = true
	need := target - len(idCodeCache.records)
	idCodeCache.Unlock()

	records, result := fetchRandomIDCodesConcurrent(need)
	if result == nil {
		idCodeCache.Lock()
		for i := range records {
			if !IsAdultIDCard(records[i].IDCard) {
				_ = DB.Delete(&IDCode{}, records[i].ID)
				continue
			}
			record := records[i]
			idCodeCache.records = append(idCodeCache.records, &record)
		}
		idCodeCache.Unlock()
	} else {
		log.Printf("实名信息缓存补充失败: %v", result)
	}

	idCodeCache.Lock()
	idCodeCache.filling = false
	target = idCodeCache.target
	if target < idCodeCacheSize {
		target = idCodeCacheSize
	}
	needMore := len(idCodeCache.records) < target
	idCodeCache.Unlock()
	if needMore && result == nil {
		// Retry later when the table has fewer valid records than the target.
		time.AfterFunc(time.Second, refillIDCodeCache)
	}
}

func GetRandomIDCode() (*IDCode, error) {
	idCodeCache.Lock()
	if len(idCodeCache.records) > 0 {
		index, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(idCodeCache.records))))
		if err != nil {
			idCodeCache.Unlock()
			return nil, err
		}
		record := *idCodeCache.records[index.Int64()]
		idCodeCache.Unlock()
		return &record, nil
	}
	idCodeCache.Unlock()
	go refillIDCodeCache()

	const maxAttempts = 100
	for i := 0; i < maxAttempts; i++ {
		records, err := fetchRandomIDCodesOnce(1)
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			break
		}
		idCode := records[0]
		if IsAdultIDCard(idCode.IDCard) {
			return &idCode, nil
		}
		_ = DeleteIDCode(idCode.ID)
	}
	return nil, fmt.Errorf("未能找到已成年的身份证信息，已尝试 %d 次", maxAttempts)
}

// ReturnIDCode returns an unused real-name record to the in-memory pool.
// It is used when registration fails before the identity is accepted.
func ReturnIDCode(idCode *IDCode) {
	if idCode == nil || idCode.ID <= 0 || strings.TrimSpace(idCode.Name) == "" || strings.TrimSpace(idCode.IDCard) == "" {
		return
	}
	idCodeCache.Lock()
	defer idCodeCache.Unlock()
	for _, record := range idCodeCache.records {
		if record != nil && record.ID == idCode.ID {
			return
		}
	}
	copy := *idCode
	idCodeCache.records = append(idCodeCache.records, &copy)
}

func fetchRandomIDCodesConcurrent(limit int) ([]IDCode, error) {
	if limit <= 0 {
		return nil, nil
	}
	type result struct {
		records []IDCode
		err     error
	}
	const workers = 64
	results := make(chan result, workers)
	for i := 0; i < workers; i++ {
		go func() {
			records, err := fetchRandomIDCodesOnce(1)
			results <- result{records: records, err: err}
		}()
	}
	var records []IDCode
	var firstErr error
	for i := 0; i < workers; i++ {
		item := <-results
		if item.err != nil && firstErr == nil {
			firstErr = item.err
		}
		records = append(records, item.records...)
	}
	if len(records) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return records, nil
}

func fetchRandomIDCodesOnce(limit int) ([]IDCode, error) {
	if limit <= 0 {
		return nil, nil
	}
	var maxID int
	if err := DB.Model(&IDCode{}).Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
		return nil, err
	}
	if maxID <= 0 {
		return nil, nil
	}
	start := 1
	if random, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(maxID))); err == nil {
		start = int(random.Int64()) + 1
	}
	var records []IDCode
	if err := DB.Where("id >= ?", start).Order("id").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) < limit {
		var wrapped []IDCode
		if err := DB.Where("id < ?", start).Order("id").Limit(limit - len(records)).Find(&wrapped).Error; err != nil {
			return nil, err
		}
		records = append(records, wrapped...)
	}
	return records, nil
}

// DeleteIDCode 删除身份证信息
func DeleteIDCode(id int) error {
	result := DB.Delete(&IDCode{}, id)
	if result.Error != nil {
		return result.Error
	}
	idCodeCache.Lock()
	defer idCodeCache.Unlock()
	for index := len(idCodeCache.records) - 1; index >= 0; index-- {
		record := idCodeCache.records[index]
		if record != nil && record.ID == id {
			idCodeCache.records = append(idCodeCache.records[:index], idCodeCache.records[index+1:]...)
		}
	}
	return nil
}

// CountIDCodes returns the remaining real-name records available to the
// registration workers.
func CountIDCodes() (int64, error) {
	var count int64
	if err := DB.Model(&IDCode{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
