package examples

import (
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/getcharzp/go-ocr/ddddocr"
	"github.com/up-zero/gotool/imageutil"
)

func TestCaptchaOcr(t *testing.T) {
	captchaURL := "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=captchaReq02ad65c21d220909027"
	savePath := "./captcha_4399.png"

	// 初始化 ddddocr 引擎
	config := ddddocr.Config{
		OnnxRuntimeLibPath: "../lib/onnxruntime.dll",
		ModelPath:          "../ddddocr_weights/common.onnx",
		DictPath:           "../ddddocr_weights/dict.txt",
	}

	engine, err := ddddocr.NewEngine(config)
	if err != nil {
		log.Fatalf("创建 OCR 引擎失败: %v\n", err)
	}
	defer engine.Destroy()

	// 下载验证码并识别，不足4位自动重试
	start := time.Now()
	for attempt := 1; ; attempt++ {
		// 下载验证码
		resp, err := http.Get(captchaURL)
		if err != nil {
			log.Fatalf("下载验证码失败: %v\n", err)
		}
		file, err := os.Create(savePath)
		if err != nil {
			resp.Body.Close()
			log.Fatalf("创建文件失败: %v\n", err)
		}
		_, err = io.Copy(file, resp.Body)
		resp.Body.Close()
		file.Close()
		if err != nil {
			log.Fatalf("保存验证码失败: %v\n", err)
		}

		// 加载图片并识别
		img, err := imageutil.Open(savePath)
		if err != nil {
			log.Fatalf("加载图像失败: %v\n", err)
		}

		result, err := engine.Classification(img)
		if err != nil {
			log.Fatalf("识别失败: %v\n", err)
		}

		t.Logf("第 %d 次尝试, 识别结果: %s (长度: %d)\n", attempt, result, len(result))

		if len(result) == 4 {
			t.Logf("识别成功! 总耗时: %v, 识别结果: %s\n", time.Since(start), result)
			break
		}
		t.Logf("结果不足4位，重新下载验证码重试...\n")
	}
}
