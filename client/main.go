package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"

	"tesi-simplepir/utils"

	"github.com/ahenzinger/simplepir/pir"
)

type ClientSetupData struct {
	Params pir.Params
	Info   pir.DBinfo
	A      pir.State
	H      pir.Msg
	K      uint64
	M      int
}

func main() {
	fmt.Println("[CLIENT] Starting...")

	fmt.Println("[CLIENT] Loading Hint...")
	hintPaths := []string{
		"client_hint.gob",
		"server/client_hint.gob",
		"../server/client_hint.gob",
	}
	var file *os.File
	var err error
	var hintPath string
	for _, path := range hintPaths {
		file, err = os.Open(path)
		if err == nil {
			hintPath = path
			break
		}
	}
	if file == nil {
		fmt.Println("[CLIENT] ERROR 'client_hint.gob' not found; run the server first")
		return
	}
	defer file.Close()
	fmt.Printf("[CLIENT] Using hint %s\n", hintPath)

	var setup ClientSetupData
	if err := gob.NewDecoder(file).Decode(&setup); err != nil {
		fmt.Printf("[CLIENT] ERROR decoding hint: %v\n", err)
		return
	}

	pi := pir.SimplePIR{}

	idx := rand.Uint64N(uint64(setup.M))
	fmt.Printf("[CLIENT] Requested idx %d\n", idx)

	cli, q := pi.Query(idx, setup.A, setup.Params, setup.Info)
	var queryBuf bytes.Buffer
	gob.NewEncoder(&queryBuf).Encode(q)

	resp, err := http.Post("http://localhost:8080/query", "application/octet-stream", &queryBuf)
	if err != nil {
		panic("Server connection error")
	}

	var ans pir.Msg
	gob.NewDecoder(resp.Body).Decode(&ans)
	resp.Body.Close()

	ansblock := utils.Unpack(utils.RecoverColumn(
		setup.Params, setup.Info, setup.H, q, ans, cli, setup.K,
	))

	jsonStr, _ := utils.DecodeMsgpackToJson(ansblock)
	fmt.Println(jsonStr)
	fmt.Println("Block Extracted")
}
