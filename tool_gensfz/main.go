package main

import (
	"fmt"
	"math/rand/v2"
)

var sfzCoeff = []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
var sfzCheck = []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}

var regions = []string{
	"510104", "510107", "500103", "110101", "310101",
	"440103", "330102", "320102", "420102", "610103",
}

func validSFZ(region, birthday string) string {
	base := region + birthday
	seq := rand.IntN(1000)
	base += fmt.Sprintf("%03d", seq)
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(base[i]-'0') * sfzCoeff[i]
	}
	return base + string(sfzCheck[sum%11])
}

func main() {
	for i := 0; i < 10; i++ {
		region := regions[rand.IntN(len(regions))]
		year := 1990 + rand.IntN(11)
		month := 1 + rand.IntN(12)
		day := 1 + rand.IntN(28)
		birthday := fmt.Sprintf("%04d%02d%02d", year, month, day)
		fmt.Printf("%s  (年龄:%d)\n", validSFZ(region, birthday), 2026-year)
	}
	fmt.Println("\n姓名: 王芳、李娜、刘洋、陈静、杨帆、赵磊、黄敏、周杰、吴婷、郑浩")
}
