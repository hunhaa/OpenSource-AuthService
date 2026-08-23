package com4399

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/getcharzp/go-ocr/ddddocr"
	"github.com/up-zero/gotool/imageutil"
)

const (
	// CaptchaExpectedLen 4399 验证码期望的字符长度
	CaptchaExpectedLen = 4
	// CaptchaMaxRetries OCR 识别最大重试次数
	CaptchaMaxRetries = 10
)

var (
	captchaEngine     *ddddocr.Engine
	captchaEngineOnce sync.Once
	captchaEngineErr  error
)

// InitCaptchaEngine 使用指定的动态链接库、模型和字典路径初始化 ddddocr 验证码识别引擎。
func InitCaptchaEngine(libPath, modelPath, dictPath string) error {
	captchaEngineOnce.Do(func() {
		config := ddddocr.Config{
			OnnxRuntimeLibPath: libPath,
			ModelPath:          modelPath,
			DictPath:           dictPath,
		}

		captchaEngine, captchaEngineErr = ddddocr.NewEngine(config)
	})
	return captchaEngineErr
}

// getCaptchaEngine 获取或初始化 ddddocr 引擎。
// 在程序运行目录（可执行文件目录、当前工作目录）下递归遍历查找所需文件：
// ONNX Runtime 动态库（按平台匹配 .dll/.so/.dylib）、common.onnx、dict.txt，
// 找到后即可初始化引擎。
func getCaptchaEngine() (*ddddocr.Engine, error) {
	if captchaEngine != nil {
		return captchaEngine, nil
	}
	// 候选根目录列表
	roots := []string{"."}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}

	required := []ocrFileSpec{
		{key: "lib", names: onnxRuntimeLibNames()},
		{key: "model", names: []string{"common.onnx"}},
		{key: "dict", names: []string{"dict.txt"}},
	}

	var lastErr error
	for _, root := range roots {
		paths := findOCRFiles(root, required)
		if len(paths) != len(required) {
			continue
		}
		if err := InitCaptchaEngine(paths["lib"], paths["model"], paths["dict"]); err != nil {
			lastErr = err
			continue
		}
		return captchaEngine, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("初始化 OCR 引擎失败: %w", lastErr)
	}
	return nil, fmt.Errorf("找不到 OCR 所需文件（%s、common.onnx、dict.txt）", strings.Join(onnxRuntimeLibNames(), " 或 "))
}

// ocrFileSpec 描述一个待查找文件的可接受文件名集合。
type ocrFileSpec struct {
	key   string   // 结果映射中的 key
	names []string // 任一文件名匹配即视为找到
}

// onnxRuntimeLibNames 返回当前平台可能的 ONNX Runtime 库文件名。
func onnxRuntimeLibNames() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"onnxruntime.dll"}
	case "linux":
		return []string{
			fmt.Sprintf("onnxruntime_%s.so", runtime.GOARCH),
			"onnxruntime.so",
			"libonnxruntime.so",
		}
	case "darwin":
		return []string{
			fmt.Sprintf("onnxruntime_%s.dylib", runtime.GOARCH),
			"onnxruntime.dylib",
			"libonnxruntime.dylib",
		}
	default:
		return []string{"onnxruntime.dll", "onnxruntime.so"}
	}
}

// findOCRFiles 在 root 目录下递归遍历，按 spec 中的文件名查找所需文件。
// 返回 spec.key -> 路径 的映射，未找到的文件不会出现在结果中。
func findOCRFiles(root string, specs []ocrFileSpec) map[string]string {
	result := make(map[string]string)
	needed := make(map[string]string)
	for _, s := range specs {
		for _, n := range s.names {
			needed[n] = s.key
		}
	}
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		name := filepath.Base(p)
		if key, ok := needed[name]; ok {
			if _, exists := result[key]; !exists {
				result[key] = p
			}
		}
		return nil
	})
	return result
}

// RecognizeCaptcha 使用 OCR 自动识别验证码。
// 下载 verifyURL 的验证码图片，使用 ddddocr 识别，
// 如果识别结果不足 4 位则重新下载并重试，最多尝试 CaptchaMaxRetries 次。
func RecognizeCaptcha(verifyURL string) (string, error) {
	return recognizeCaptchaWithHTTPClient(verifyURL, http.DefaultClient)
}

// RecognizeCaptchaWithHTTPClient 通过 URL + 指定 HTTP client 识别验证码
func RecognizeCaptchaWithHTTPClient(verifyURL string, client *http.Client) (string, error) {
	return recognizeCaptchaWithHTTPClient(verifyURL, client)
}

// RecognizeCaptchaBytes 直接从内存字节识别验证码（适用于已下载到内存中的验证码图片）
func RecognizeCaptchaBytes(imgBytes []byte) (string, error) {
	eng, err := getCaptchaEngine()
	if err != nil {
		return "", err
	}
	tmpFile, err := os.CreateTemp("", "captcha_bytes_*.png")
	if err != nil {
		return "", fmt.Errorf("com4399: 创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.Write(imgBytes); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("com4399: 写入临时文件失败: %w", err)
	}
	tmpFile.Close()
	img, err := imageutil.Open(tmpPath)
	if err != nil {
		return "", fmt.Errorf("com4399: 加载验证码图片失败: %w", err)
	}
	result, err := eng.Classification(img)
	if err != nil {
		return "", fmt.Errorf("com4399: 识别验证码失败: %w", err)
	}
	result = strings.TrimSpace(result)
	log.Printf("[OCR] 直接识别字节结果: %s (长度: %d)", result, len(result))
	return result, nil
}

func recognizeCaptchaWithHTTPClient(verifyURL string, client *http.Client) (string, error) {
	eng, err := getCaptchaEngine()
	if err != nil {
		return "", err
	}

	verifyURL = strings.TrimSpace(verifyURL)
	if verifyURL == "" {
		return "", fmt.Errorf("com4399: 验证码 URL 为空")
	}

	for attempt := 1; attempt <= CaptchaMaxRetries; attempt++ {
		imgPath, err := downloadCaptchaImageWithHTTPClient(verifyURL, client)
		if err != nil {
			return "", err
		}

		img, err := imageutil.Open(imgPath)
		os.Remove(imgPath)
		if err != nil {
			return "", fmt.Errorf("com4399: 加载验证码图片失败: %w", err)
		}

		result, err := eng.Classification(img)
		if err != nil {
			return "", fmt.Errorf("com4399: 识别验证码失败: %w", err)
		}

		result = strings.TrimSpace(result)
		log.Printf("[OCR] 第 %d 次尝试, 识别结果: %s (长度: %d)", attempt, result, len(result))

		if len(result) == CaptchaExpectedLen {
			return result, nil
		}

		log.Printf("[OCR] 结果不足 %d 位，重新下载验证码重试...", CaptchaExpectedLen)
	}

	return "", fmt.Errorf("com4399: OCR 识别验证码失败: 超过最大重试次数 %d", CaptchaMaxRetries)
}

// DestroyCaptchaEngine 释放 OCR 引擎资源。
func DestroyCaptchaEngine() {
	if captchaEngine != nil {
		captchaEngine.Destroy()
		captchaEngine = nil
	}
}

// downloadCaptchaImage 从 URL 下载验证码图片并保存到临时文件，返回路径。
func downloadCaptchaImageWithHTTPClient(verifyURL string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequest(http.MethodGet, verifyURL, nil)
	if err != nil {
		return "", fmt.Errorf("com4399: create captcha request: %w", err)
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Referer", ucRegisterFrameURL)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("com4399: 下载验证码图片失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("com4399: 下载验证码图片失败: HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "captcha_*.png")
	if err != nil {
		return "", fmt.Errorf("com4399: 创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("com4399: 保存验证码图片失败: %w", err)
	}

	return tmpPath, nil
}
