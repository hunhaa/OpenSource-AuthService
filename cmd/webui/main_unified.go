package main

import (
	"os"

	unified "github.com/Yeah114/FunAuth/internal/unified"
)

func main() {
	unified.RunAsFlavor("webui", os.Args[1:])
}
