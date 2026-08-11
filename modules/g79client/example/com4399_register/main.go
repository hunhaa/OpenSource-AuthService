package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yeah114/g79client/account/com4399"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	req := com4399.WebRegisterRequest{
		Username: strings.TrimSpace(os.Getenv("G79_4399_USERNAME")),
		Password: os.Getenv("G79_4399_PASSWORD"),
		RealName: strings.TrimSpace(os.Getenv("G79_4399_REAL_NAME")),
		IDCard:   strings.TrimSpace(os.Getenv("G79_4399_ID_CARD")),
	}
	if req.Username == "" {
		req.Username = readLine(reader, "请输入要注册的 4399 用户名: ")
	}
	if req.Password == "" {
		req.Password = readLine(reader, "请输入要注册的 4399 密码: ")
	}
	if req.RealName == "" {
		req.RealName = readLine(reader, "请输入实名认证姓名: ")
	}
	if req.IDCard == "" {
		req.IDCard = readLine(reader, "请输入实名认证身份证号: ")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := com4399.NewWebRegisterClient(nil)
	result, err := registerWithCaptcha(ctx, client, reader, req)
	if err != nil {
		log.Fatalf("4399 注册失败: %v", err)
	}

	fmt.Println("4399 注册成功，Cookie:")
	fmt.Println(result.Cookie)

	pretty, err := json.MarshalIndent(result.Register, "", "  ")
	if err == nil {
		fmt.Println("注册结果:")
		fmt.Println(string(pretty))
	}
}

func registerWithCaptcha(ctx context.Context, client *com4399.WebRegisterClient, reader *bufio.Reader, req com4399.WebRegisterRequest) (*com4399.WebRegisterCookieResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		result, err := client.RegisterAndLoginCookie(ctx, req)
		if err == nil {
			return result, nil
		}
		var captchaErr *com4399.WebCaptchaRequiredError
		if !errors.As(err, &captchaErr) {
			return nil, err
		}
		path := filepath.Join(os.TempDir(), "4399-register-captcha.png")
		if captchaErr.CaptchaID != "" {
			path = filepath.Join(os.TempDir(), "4399-register-captcha-"+captchaErr.CaptchaID+".png")
		}
		if captchaErr.CaptchaURL != "" {
			if saveErr := client.SaveCaptchaImage(ctx, captchaErr.CaptchaURL, path); saveErr != nil {
				return nil, fmt.Errorf("%w; 保存验证码失败: %v", err, saveErr)
			}
			fmt.Printf("检测到验证码，图片已保存: %s\n", path)
		}
		req.CaptchaCode = readLine(reader, "请输入验证码: ")
		if req.CaptchaCode == "" {
			return nil, err
		}
	}
	return nil, fmt.Errorf("验证码重试次数过多")
}

func readLine(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	value, _ := reader.ReadString('\n')
	return strings.TrimSpace(value)
}
