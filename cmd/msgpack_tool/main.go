package main

import (
	"fmt"
	"os"

	"tesi-simplepir/utils"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/msgpack_tool <file>")
		os.Exit(1)
	}

	filePath := os.Args[1]
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	jsonStr, err := utils.DecodeMsgpackToJson(fileData)
	if err != nil {
		fmt.Printf("Error decoding: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(jsonStr)
}
