package main

import (
	"os"

	// 这里直接复用 funauth 主包的 CLI 分发。为避免 "import cycle"，我们让 funauth
	// 导出一个入口函数，然后 webui 就调用它，设置默认 flavor = "webui"。
	funauthcli "github.com/Yeah114/FunAuth/cmd/funauth"
)

func main() {
	funauthcli.RunAsFlavor("webui", os.Args[1:])
}
