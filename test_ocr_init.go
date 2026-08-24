//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Yeah114/g79client/account/com4399"
)

func main() {
	log.Printf("===== OCR 引擎初始化验证 =====")
	log.Printf("资源路径: ./ocr_resources/")

	// 检查文件
	for _, f := range []string{"common.onnx", "dict.txt", "libonnxruntime.so"} {
		st, err := os.Stat("./ocr_resources/" + f)
		if err != nil {
			log.Printf("  ✗ ./ocr_resources/%s  缺失: %v", f, err)
		} else {
			log.Printf("  ✓ ./ocr_resources/%s  (%d bytes)", f, st.Size())
		}
	}
	fmt.Println()

	// 尝试识别一张假图片（空字节或PNG头），关键是看引擎能不能加载
	// 失败如果是 "image format" / "太小" 之类的，说明引擎已经成功初始化
	// 失败如果是 "ApiVersion" / "not ready" / "onnxruntime" 之类的，说明初始化失败
	fake := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG magic
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
	}

	log.Printf("尝试 InitCaptchaEngine + RecognizeCaptchaBytes（用假图，只验证引擎能否加载）...")
	text, err := com4399.RecognizeCaptchaBytes(fake)
	fmt.Println()
	fmt.Println("============= 结果 =============")
	fmt.Printf("识别结果 text=%q\n", text)
	if err != nil {
		fmt.Printf("error = %v\n", err)
		estr := fmt.Sprintf("%v", err)
		switch {
		case ci(estr, "ApiVersion") || ci(estr, "api version") || ci(estr, "not available"):
			fmt.Println("❌ 仍然是 ApiVersion 不匹配 → OCR 引擎初始化失败")
		case ci(estr, "engine") || ci(estr, "not ready") || ci(estr, "ErrCaptchaEngineNotReady"):
			fmt.Println("❌ 引擎未就绪 → 初始化失败")
		case ci(estr, "onnxruntime") || ci(estr, "onnx") || ci(estr, "libonnxruntime"):
			fmt.Println("❌ libonnxruntime.so 加载/版本问题")
		case ci(estr, "decode") || ci(estr, "format") || ci(estr, "太小") || ci(estr, "image") || ci(estr, "png") || ci(estr, "captcha"):
			fmt.Println("✅ 引擎成功初始化！（失败是因为假图片，不是引擎本身问题）")
		case ci(estr, "dict") || ci(estr, "model") || ci(estr, "common.onnx") || ci(estr, "找不到") || ci(estr, "missing"):
			fmt.Println("❌ 资源文件缺失或路径错")
		default:
			fmt.Println("⚠️  其他错误，请仔细判断：", estr)
		}
	} else {
		fmt.Printf("✅ 引擎初始化成功！假图片居然识别出了 %q？\n", text)
	}
}

func ci(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	ls := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		ls[i] = c
	}
	lsub := make([]byte, len(sub))
	for i := 0; i < len(sub); i++ {
		c := sub[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		lsub[i] = c
	}
	n, m := len(ls), len(lsub)
	for i := 0; i+m <= n; i++ {
		match := true
		for j := 0; j < m; j++ {
			if ls[i+j] != lsub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
