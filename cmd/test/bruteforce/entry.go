package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Yeah114/g79client"
)

// Main 作为 funauth test bruteforce 子命令入口（同时保留原二进制独立运行）。
func Main(args []string) {
	runBruteForce(args)
}

func main() {
	Main(os.Args[1:])
}

const sauth1 = `{"gameid": "x19", "login_channel": "netease", "app_channel": "netease", "platform": "pc", "sdkuid": "aibgvakgg6ozaucj", "sessionid": "1-eyJzaSI6ICIwOGFjMzdlYjJjMGZiODRhYTAxYjFhMDgzODExYjc1YTliYTU4N2U2IiwgIm9kaSI6ICJhbWF3dWZ5YWF4dHUzdWZxLWQiLCAicyI6ICI4ZXdlYzR4NTU2Zjc0Y2syd2FubnE5ZmZoaXIzaDRoMSIsICJ1IjogImFpYmd2YWtnZzZvemF1Y2oiLCAidCI6IDIsICJwcnMiOiAxMjgsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCIsICJzYSI6IC05OX0g", "sdk_version": "5.9.0", "udid": "amawufyaaxtu3ufq-d", "deviceid": "amawufyaaxtu3ufq-d", "aim_info": "{\"aim\": \"127.0.0.1\", \"country\": \"CN\", \"tz\": \"+0800\", \"tzid\": \"\"}", "client_login_sn": "1b098d080b7d28b80f27445fa86a5998", "gas_token": "", "source_platform": "netease", "ip": "127.0.0.1", "nickname": "Nan_4504o"}`

func cookieStr() string {
	cd := map[string]string{"sauth_json": sauth1}
	b, _ := json.Marshal(cd)
	return string(b)
}

func runBruteForce(args []string) {
	_ = args
	log.SetOutput(os.Stdout)
	cs := cookieStr()
	found := false
	for sp := 0; sp <= 15; sp++ {
		for tr := 1; tr <= 20; tr++ {
			client, err := g79client.NewClient()
			if err != nil {
				log.Fatalf("NewClient fail: %v", err)
			}
			err = client.G79AuthenticateWithCookieTest(cs, sp, tr)
			if err == nil {
				fmt.Printf("✅✅✅ SUCCESS: sp=%d, tr=%d, uid=%s token[:20]=%s\n",
					sp, tr, client.UserID, truncate(client.UserToken, 20))
				found = true
				return
			}
			msg := err.Error()
			if strings.Contains(msg, "code: 2001") {
				fmt.Printf("sp=%2d tr=%2d → 2001版本过低\n", sp, tr)
			} else if strings.Contains(msg, "code:") {
				fmt.Printf("sp=%2d tr=%2d → ERR: %s\n", sp, tr, firstLine(msg))
			} else {
				fmt.Printf("sp=%2d tr=%2d → OTHER: %s\n", sp, tr, firstLine(msg))
			}
			time.Sleep(120 * time.Millisecond)
		}
	}
	if !found {
		fmt.Println("\n❌ 穷举sp=0..15, tr=1..20全部失败，可能需要修改PeAuthSign算法或版本参数")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
