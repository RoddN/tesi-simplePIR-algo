package main

import (
	"fmt"
	"os"

	"tesi-simplepir/blockpir"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: go run ./tools/msgpack2json <file>")
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Errore:", err)
		os.Exit(1)
	}

	text, err := blockpir.MsgpackToJSON(data)
	if err != nil {
		fmt.Println("Errore:", err)
		os.Exit(1)
	}
	fmt.Println(text)
}
