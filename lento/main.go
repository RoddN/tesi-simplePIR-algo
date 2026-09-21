package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ahenzinger/simplepir/pir"
)

const d = uint64(64)

func read_blocks(dir string, threshold int) []string {
	var blocks []string

	file, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	for _, f := range file {
		content, err := os.ReadFile(dir + "/" + f.Name())
		if err != nil {
			panic(err)
		}
		if len(content) > threshold {
			continue
		}
		blocks = append(blocks, string(content))
	}
	return blocks
}

func find_K(blocks []string) uint64 {
	max_len := 0
	for _, block := range blocks {
		if len(block) > max_len {
			max_len = len(block)
		}
	}
	return uint64((max_len + 7) / 8)
}

func text_to_num(chunk string) uint64 {
	value := uint64(0)
	for _, ch := range chunk {
		value = value*256 + uint64(ch)
	}
	return value
}

func num_to_text(value uint64) string {
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = byte(value % 256)
		value = value / 256
	}
	return string(buf)
}

func block_round(block string) uint64 {
	var b struct {
		Block struct {
			Rnd uint64 `json:"rnd"`
		} `json:"block"`
	}
	if err := json.Unmarshal([]byte(block), &b); err != nil {
		panic(err)
	}
	return b.Block.Rnd
}

func make_DB(pi pir.SimplePIR, blocks []string) (*pir.Database, pir.Params, uint64, uint64) {
	M := uint64(len(blocks))
	K := find_K(blocks)
	N := K * M

	p := pi.PickParams(N, d, 1024, 32)

	data := make([]uint64, N)
	width := int(K) * 8
	for j, block := range blocks {
		padded := fmt.Sprintf("%-*s", width, block)
		for c := uint64(0); c < K; c++ {
			chunk := padded[c*8 : (c+1)*8]
			data[uint64(j)*K+c] = text_to_num(chunk)
		}
	}

	DB := pir.MakeDB(N, d, &p, data)
	return DB, p, K, M
}

func query_entry(pi pir.SimplePIR, DB *pir.Database, A pir.State, serverState pir.State,
	H pir.Msg, p pir.Params, index uint64) uint64 {

	s, query := pi.Query(index, A, p, DB.Info)

	var batch pir.MsgSlice
	batch.Data = append(batch.Data, query)
	answer := pi.Answer(DB, batch, serverState, A, p)

	return pi.Recover(index, 0, H, query, answer, A, s, p, DB.Info)
}

func query_block(pi pir.SimplePIR, DB *pir.Database, A pir.State, serverState pir.State,
	H pir.Msg, p pir.Params, K uint64, M uint64, j uint64) string {

	full := ""
	for c := uint64(0); c < K; c++ {
		value := query_entry(pi, DB, A, serverState, H, p, j*K+c)
		full += num_to_text(value)
	}
	return strings.TrimRight(full, " ")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: lento <indice blocco> [soglia byte]")
		os.Exit(1)
	}
	index, err := strconv.ParseUint(os.Args[1], 10, 64)
	if err != nil {
		panic(err)
	}

	threshold := 32768
	if len(os.Args) > 2 {
		threshold, err = strconv.Atoi(os.Args[2])
		if err != nil {
			panic(err)
		}
	}

	blocks := read_blocks("blocchi", threshold)
	fmt.Fprintf(os.Stderr, "blocchi caricati: %d\n", len(blocks))

	pi := pir.SimplePIR{}

	start := time.Now()
	DB, p, K, M := make_DB(pi, blocks)
	A := pi.Init(DB.Info, p)
	serverState, H := pi.Setup(DB, A, p)
	fmt.Fprintf(os.Stderr, "K=%d M=%d L=%d (matrice %dx%d)  setup: %v\n",
		K, M, p.L, p.L, p.M, time.Since(start))

	start = time.Now()
	block := query_block(pi, DB, A, serverState, H, p, K, M, index)
	fmt.Fprintf(os.Stderr, "query (%d chiamate): %v\n", K, time.Since(start))

	esito := "OK"
	if block != blocks[index] {
		esito = "ERRORE"
	}
	fmt.Printf("indice %d -> round %d, %d byte, %s\n", index, block_round(block), len(block), esito)
}
