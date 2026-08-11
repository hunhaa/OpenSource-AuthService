package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: decrypt <hex_string> 或 decrypt -f <file>")
		os.Exit(1)
	}

	var hexData string

	data, err := os.ReadFile(os.Args[1])
	if err == nil {
		hexData = string(data)
	} else {
		hexData = os.Args[1]
	}

	hexData = strings.TrimSpace(hexData)

	bytes, err := hex.DecodeString(hexData)
	if err != nil {
		fmt.Printf("Hex解码失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("数据长度: %d 字节\n", len(bytes))
	fmt.Printf("最后1字节: 0x%02X\n", bytes[len(bytes)-1])

	info := bytes[len(bytes)-1]
	version := info & 0x0F
	index := info >> 4

	fmt.Printf("解析 - version: %d, key_index: %d\n", version, index)

	if len(bytes) < 17 {
		fmt.Println("数据太短")
		os.Exit(1)
	}

	iv := bytes[:16]
	encrypted := bytes[16 : len(bytes)-1]

	fmt.Printf("IV长度: %d, 密文长度: %d\n", len(iv), len(encrypted))

	keys := []string{
		"1C8D9CAD811F2F1E3F7B3B5D208DBE83",
		"96271EAE017F444B342EB8C53AE106BA",
		"6B5FF1292C60237E776923CF4C49ED9A",
		"F020969C883F9C7ADC2C5A130FD1A9CC",
		"607EB25E875E8C9B5C1C5D08648F2D8F",
		"E67D0B4397CCB39C97A78F03315FBE03",
		"3F8F5F7CC35950BF5C9E2B56B20A1F1B",
		"7B8D1D7D674D7D8D917F7D0F2A4D9B48",
		"78289B0C06234B2A2A7D9D7C3C1C1CD3",
		"180928C6AF6C703FBD1A1C8C7EB06C8F",
		"9D2B5D4D0C843E445C4E2A3E19CCCC3F",
		"ED33B9EF6DCC3D2D45428AF516F80A4A",
		"7B6CBC5C215E3E4B7E2F5F5A206823DD",
		"7932137D6A2E1B7E204D6D891ED55D95",
		"0E516BF59BEE9EB6712F5E5D63AC89D2",
		"ED8BBB2D808A0D8F353F3F7B0E486BE5",
	}

	if int(index) >= len(keys) {
		fmt.Printf("Key索引 %d 超出范围\n", index)
		os.Exit(1)
	}

	keyHex := keys[index]
	keyBytes, _ := hex.DecodeString(keyHex)

	for i := range keyBytes {
		keyBytes[i] ^= 0x7C
	}

	fmt.Printf("使用密钥: %s\n", keyHex)

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		fmt.Printf("创建Cipher失败: %v\n", err)
		os.Exit(1)
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encrypted))
	mode.CryptBlocks(decrypted, encrypted)

	dropPos := len(decrypted) - 1
	for dropPos >= 0 && decrypted[dropPos] == 0 {
		dropPos--
	}
	dropPos -= 16

	if dropPos < 0 {
		dropPos = 0
	}

	fmt.Println("\n解密结果:")
	fmt.Println(string(decrypted[:dropPos+1]))
}
