package login

import (
	"fmt"

	"github.com/Yeah114/g79client"
	examplecookie "github.com/Yeah114/g79client/example/cookie"
)

func Cookie() string {
	return examplecookie.Value()
}

func Login() (*g79client.Client, error) {
	cookie := Cookie()

	client, err := g79client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("创建客户端失败: %v", err)
	}

	// 认证
	if err := client.G79AuthenticateWithCookie(cookie); err != nil {
		return nil, fmt.Errorf("认证失败: %v", err)
	}

	return client, nil
}
