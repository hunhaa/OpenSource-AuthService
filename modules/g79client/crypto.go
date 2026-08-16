package g79client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// AES密钥列表
var keys = []string{
	"60F1E0D1FD635362430747215CF1C2FF",
	"EA5B62D27D0338374852C4B9469D7AC6",
	"17238D55501C5F020B155FB3303591E6",
	"8C5CEAE0F443E006A050266F73ADD5B0",
	"1C02CE22FB22F0E72060217418F351F3",
	"9A01773FEBB0CFE0EBDBF37F4D23C27F",
	"43F32300BF252CC320E2572ACE766367",
	"07F161011B3101F1ED0301735631E734",
	"0454E7707A5F37565601E100406060AF",
	"647554BAD3100C43C16660F002CC10F3",
	"E157213170F842382032564265B0B043",
	"914FC59311B04151393EF6896A847636",
	"0710C0205D224237025323265C145FA1",
	"054E6F01165267025C3111F562A921E9",
	"722D1789E792E2CA0D5322211FD0F5AE",
	"91F7C751FCF671F34943430772341799",
}

var x19Keys = []string{
	"MK6mipwmOUedplb6",
	"OtEylfId6dyhrfdn",
	"VNbhn5mvUaQaeOo9",
	"bIEoQGQYjKd02U0J",
	"fuaJrPwaH2cfXXLP",
	"LEkdyiroouKQ4XN1",
	"jM1h27H4UROu427W",
	"DhReQada7gZybTDk",
	"ZGXfpSTYUvcdKqdY",
	"AZwKf7MWZrJpGR5W",
	"amuvbcHw38TcSyPU",
	"SI4QotspbjhyFdT0",
	"VP4dhjKnDGlSJtbB",
	"UXDZx4KhZywQ2tcn",
	"NIK73ZNvNqzva4kd",
	"WeiW7qU766Q1YQZI",
}

// 生成随机字符串
func randomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

func randomBytes(length int) ([]byte, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// HTTP加密 - 严格按照nethard-core TypeScript版本实现
// 格式: AES-128-CBC( body + "\n" + 16位随机字符 + 零填充至16字节倍数 )
//       输出: 16字节随机IV + 密文 + 1字节标记 (index<<4 | V4)
func G79HttpEncrypt(body []byte) ([]byte, error) {
	// 16位随机字母数字填充
	randFill, err := randomString(16)
	if err != nil {
		return nil, err
	}
	// TypeScript: `${bodyIn}\n${randFill}`
	unpadded := append([]byte{}, body...)
	unpadded = append(unpadded, '\n')
	unpadded = append(unpadded, []byte(randFill)...)

	// 填充至16字节倍数，补0
	paddedLen := len(unpadded)
	if rem := paddedLen % aes.BlockSize; rem != 0 {
		paddedLen += aes.BlockSize - rem
	}
	padded := make([]byte, paddedLen)
	copy(padded, unpadded)

	// 标记位: 随机index 0..13 << 4 | V4(0x04)
	index, err := rand.Int(rand.Reader, big.NewInt(14)) // exclusive upper bound 14 => 0..13
	if err != nil {
		return nil, err
	}
	flag := byte((index.Int64() << 4) | 0x04)

	// 16字节随机IV
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	keyIndex := (flag >> 4) & 0x0F
	keyBytes, err := hex.DecodeString(keys[keyIndex])
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)

	out := make([]byte, 0, len(iv)+len(encrypted)+1)
	out = append(out, iv...)
	out = append(out, encrypted...)
	out = append(out, flag)
	return out, nil
}

// HTTP解密 - 严格按照nethard-core TypeScript版本实现
func G79HttpDecrypt(payload []byte) ([]byte, error) {
	if len(payload) < aes.BlockSize+2 { // 16 IV + 至少1字节数据 + 1字节flag
		return nil, fmt.Errorf("payload too short")
	}
	flag := payload[len(payload)-1]
	keyIdentifier := flag & 0x0F
	if keyIdentifier != 0x04 && keyIdentifier != 0x0C { // 兼容V4和旧的V12
		return nil, fmt.Errorf("unsupported key identifier 0x%x", keyIdentifier)
	}
	keyIndex := (flag >> 4) & 0x0F
	if int(keyIndex) >= len(keys) {
		return nil, fmt.Errorf("key index %d out of range", keyIndex)
	}
	keyBytes, err := hex.DecodeString(keys[keyIndex])
	if err != nil {
		return nil, err
	}
	iv := payload[:aes.BlockSize]
	cipherText := payload[aes.BlockSize : len(payload)-1]

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, cipherText)

	// 去除尾部0填充
	end := len(plain) - 1
	for end >= 0 && plain[end] == 0 {
		end--
	}
	if end < 0 {
		return nil, fmt.Errorf("decrypted data is empty")
	}
	trimmed := plain[:end+1]

	// TypeScript: `.subarray(0, -16)` — 去掉最后16字节randFill
	if len(trimmed) < 16 {
		return nil, fmt.Errorf("decrypted content shorter than randFill")
	}
	result := trimmed[:len(trimmed)-16]

	// 如果末尾是换行符，直接保留即可（JSON解析能容忍）
	return result, nil
}

// 计算动态token
func CalculateDynamicToken(path, content, token string) string {
	salt := "0eGsBkhl"

	// 计算token的MD5
	tokenMd5 := fmt.Sprintf("%x", md5.Sum([]byte(token)))

	// 构建payload
	payload := tokenMd5 + content + salt + strings.TrimSuffix(path, "?")

	// 计算payload的MD5
	payloadMd5 := fmt.Sprintf("%x", md5.Sum([]byte(payload)))

	// 转换为二进制字符串
	binaryString := ""
	for _, b := range []byte(payloadMd5) {
		binaryString += fmt.Sprintf("%08b", b)
	}

	// 左移6位
	binaryString = binaryString[6:] + binaryString[:6]

	// 对每个字节进行位反转并异或
	transformed := make([]byte, len(payloadMd5))
	copy(transformed, []byte(payloadMd5))

	for i := 0; i < len(transformed); i++ {
		section := binaryString[i*8 : i*8+8]
		reversedByte := byte(0)
		for j := 0; j < 8; j++ {
			if section[7-j] == '1' {
				reversedByte |= (1 << (j & 0x1F))
			}
		}
		transformed[i] = reversedByte ^ transformed[i]
	}

	// Base64编码并处理
	b64 := base64.StdEncoding.EncodeToString(transformed)
	tokenShort := strings.ReplaceAll(strings.ReplaceAll(b64[:16], "+", "m"), "/", "o") + "1"

	return tokenShort
}

// X19HttpEncrypt implements the PC launcher encryption used by the X19 endpoints.
func X19HttpEncrypt(body []byte) ([]byte, error) {
	content := make([]byte, len(body))
	copy(content, body)

	tail, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	content = append(content, tail...)

	keyIndexBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(x19Keys))))
	if err != nil {
		return nil, err
	}
	keyIndex := int(keyIndexBig.Int64())

	iv, err := randomBytes(16)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher([]byte(x19Keys[keyIndex]))
	if err != nil {
		return nil, err
	}

	padded := pkcs7Pad(content, aes.BlockSize)
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)

	result := make([]byte, 0, len(iv)+len(encrypted)+1)
	result = append(result, iv...)
	result = append(result, encrypted...)
	result = append(result, byte(keyIndex<<4)|0x02)
	return result, nil
}

// X19HttpDecrypt decrypts payloads produced by X19HttpEncrypt.
func X19HttpDecrypt(payload []byte) ([]byte, error) {
	if len(payload) < 18 {
		return nil, fmt.Errorf("payload too short")
	}

	flag := payload[len(payload)-1]
	keyIndex := int((flag >> 4) & 0x0F)
	if keyIndex >= len(x19Keys) {
		return nil, fmt.Errorf("invalid key index %d", keyIndex)
	}

	iv := payload[:16]
	cipherText := payload[16 : len(payload)-1]

	block, err := aes.NewCipher([]byte(x19Keys[keyIndex]))
	if err != nil {
		return nil, err
	}

	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, cipherText)
	return plain, nil
}

// X19CalculateDynamicToken reproduces the PC launcher token logic.
func X19CalculateDynamicToken(url, content, token string) (string, error) {
	tokenMD5 := fmt.Sprintf("%x", md5.Sum([]byte(token)))
	magicMD5 := fmt.Sprintf("%x", md5.Sum([]byte(tokenMD5+content+"0eGsBkhl"+url)))

	binaryMagic := stringToBin(magicMD5)
	shifted := stringLeftShift(binaryMagic, 6)
	shiftedStr, err := binToString(shifted)
	if err != nil {
		return "", err
	}

	xorBytes, err := stringXOR(magicMD5, shiftedStr)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(xorBytes)
	replaced := strings.ReplaceAll(strings.ReplaceAll(encoded, "/", "o"), "+", "m")
	if len(replaced) < 16 {
		return "", fmt.Errorf("token too short")
	}
	return replaced[:16] + "1", nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	if padLen == 0 {
		padLen = blockSize
	}
	padding := bytesRepeat(byte(padLen), padLen)
	return append(data, padding...)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("invalid padding size")
	}
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, fmt.Errorf("invalid padding content")
		}
	}
	return data[:len(data)-padLen], nil
}

func bytesRepeat(b byte, count int) []byte {
	res := make([]byte, count)
	for i := range res {
		res[i] = b
	}
	return res
}

func stringToBin(s string) string {
	var builder strings.Builder
	for i := 0; i < len(s); i++ {
		builder.WriteString(fmt.Sprintf("%08b", s[i]))
	}
	return builder.String()
}

func stringLeftShift(s string, n int) string {
	if len(s) == 0 {
		return s
	}
	n = n % len(s)
	return s[n:] + s[:n]
}

func binToString(bin string) (string, error) {
	if len(bin)%8 != 0 {
		return "", fmt.Errorf("binary length must be multiple of 8")
	}
	result := make([]byte, len(bin)/8)
	for i := 0; i < len(result); i++ {
		chunk := bin[i*8 : (i+1)*8]
		value, err := strconv.ParseUint(chunk, 2, 8)
		if err != nil {
			return "", err
		}
		result[i] = byte(value)
	}
	return string(result), nil
}

func stringXOR(a, b string) ([]byte, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("length mismatch")
	}
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i]
	}
	return out, nil
}

// 获取有效JSON边界
func GetValidJSON(decryptedBody []byte) []byte {
	blocksCount := 0
	readableLength := 0

	for i, b := range decryptedBody {
		switch b {
		case 0x7B: // '{'
			blocksCount++
		case 0x7D: // '}'
			blocksCount--
		}

		if blocksCount == 0 && i != 0 {
			readableLength = i + 1
			break
		}
	}

	if readableLength != 0 {
		return decryptedBody[:readableLength]
	}
	return decryptedBody
}
