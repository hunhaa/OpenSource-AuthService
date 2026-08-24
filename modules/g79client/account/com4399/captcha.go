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

// ErrCaptchaEngineNotReady OCR 引擎未就绪（缺少 onnxruntime / 模型 / 字典）时返回。
// 调用方收到此错误应放弃本次请求，不要生成随机验证码（否则会触发 4399 风控）。
var ErrCaptchaEngineNotReady = fmt.Errorf("com4399: OCR 引擎不可用（缺少 onnxruntime 共享库 / common.onnx 模型 / dict.txt 字典）")

// InitCaptchaEngine 使用指定的动态链接库、模型和字典路径初始化 ddddocr 验证码识别引擎。
// useCustomModel=true 适配 boluoreg 的 4399ocr.onnx（CTC 输出节点 output，int64）
func InitCaptchaEngine(libPath, modelPath, dictPath string, useCustomModel ...bool) error {
	captchaEngineOnce.Do(func() {
		custom := true // 默认走自定义模型，匹配 boluoreg 4399ocr.onnx
		if len(useCustomModel) > 0 {
			custom = useCustomModel[0]
		}
		config := ddddocr.Config{
			OnnxRuntimeLibPath: libPath,
			ModelPath:          modelPath,
			DictPath:           dictPath,
			UseCustomModel:     custom,
		}

		captchaEngine, captchaEngineErr = ddddocr.NewEngine(config)
		if captchaEngineErr == nil {
			log.Printf("[4399-OCR] 引擎初始化成功 lib=%s model=%s dict=%s (custom=%v)",
				filepath.Base(libPath), filepath.Base(modelPath), filepath.Base(dictPath), custom)
		}
	})
	return captchaEngineErr
}

// getCaptchaEngine 获取或初始化 ddddocr 引擎。
// 搜索顺序：
//  1. ./ocr_resources/ （推荐，便于打包分发）
//  2. <可执行文件目录>/ocr_resources/
//  3. 从 cwd 递归搜索（向后兼容）
//  4. 从 exe 目录递归搜索（向后兼容）
func getCaptchaEngine() (*ddddocr.Engine, error) {
	if captchaEngine != nil {
		return captchaEngine, nil
	}
	var roots []string
	push := func(p string) {
		if p != "" {
			roots = append(roots, p)
		}
	}
	// 第一优先级：ocr_resources 目录
	push("./ocr_resources")
	if exe, err := os.Executable(); err == nil {
		push(filepath.Join(filepath.Dir(exe), "ocr_resources"))
	}
	// 第二优先级：从 cwd、exe 根目录递归搜索
	push(".")
	if cwd, err := os.Getwd(); err == nil {
		push(cwd)
	}
	if exe, err := os.Executable(); err == nil {
		push(filepath.Dir(exe))
	}

	required := []ocrFileSpec{
		{key: "lib", names: onnxRuntimeLibNames()},
		{key: "model", names: []string{"common.onnx"}},
		{key: "dict", names: []string{"dict.txt"}},
	}

	var lastErr error
	var searched []string
	for i, root := range roots {
		paths := findOCRFiles(root, required)
		searched = append(searched, fmt.Sprintf("[%d]%s(found=%d/3)", i, root, len(paths)))
		if len(paths) != len(required) {
			continue
		}
		if err := InitCaptchaEngine(paths["lib"], paths["model"], paths["dict"]); err != nil {
			lastErr = err
			log.Printf("[4399-OCR] 在 %s 找到文件但初始化失败: %v", root, err)
			continue
		}
		return captchaEngine, nil
	}
	log.Printf("[4399-OCR] 未找到 OCR 文件，搜索路径: %s", strings.Join(searched, " "))
	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrCaptchaEngineNotReady, lastErr)
	}
	return nil, ErrCaptchaEngineNotReady
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
