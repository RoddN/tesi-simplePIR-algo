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

// K = numero di entry da 8 byte necessarie per il blocco piu' lungo.
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

// un numero a 64 bit -> 8 caratteri.
func num_to_text(value uint64) string {
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = byte(value % 256)
		value = value / 256
	}
	return string(buf)
}

// Legge il round dal JSON del blocco.
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

	// L provvisorio = 1: serve solo per ottenere il modulo p.P dalla tabella.
	p := pi.PickParamsGivenDimensions(1, M, 1024, 32)

	// Ne = quante celle mod p servono per una entry da 64 bit (7 con p=991).
	// L'altezza reale della matrice e' K entry * Ne celle.
	_, Ne, _ := pir.Num_DB_entries(N, d, p.P)
	p.L = K * Ne

	// Riempimento: MakeDB mette la entry i in riga i/M, colonna i%M,
	// quindi la posizione c*M + j finisce in riga c, colonna j.
	data := make([]uint64, N)
	width := int(K) * 8
	for j, block := range blocks {
		padded := fmt.Sprintf("%-*s", width, block)
		for c := uint64(0); c < K; c++ {
			chunk := padded[c*8 : (c+1)*8]
			data[c*M+uint64(j)] = text_to_num(chunk)
		}
	}

	DB := pir.MakeDB(N, d, &p, data)
	return DB, p, K, M
}

// Come pi.Recover, ma decodifica tutte le K entry della colonna j
// dalla stessa risposta, invece di una sola riga.
func recover_column(p pir.Params, info pir.DBinfo, H pir.Msg, query pir.Msg,
	answer pir.Msg, s pir.State, K uint64, M uint64, j uint64) []uint64 {

	secret := s.Data[0]
	hint := H.Data[0]
	ans := answer.Data[0]

	// Stesso offset che calcola pi.Recover.
	ratio := p.P / 2
	offset := uint64(0)
	for i := uint64(0); i < p.M; i++ {
		offset += ratio * query.Data[0].Get(i, 0)
	}
	offset %= (1 << p.Logq)
	offset = (1 << p.Logq) - offset

	// ans - H*s = Delta * (colonna j) + rumore
	interm := pir.MatrixMul(hint, secret)
	ans.MatrixSub(interm)

	entries := make([]uint64, K)
	for c := uint64(0); c < K; c++ {
		// la entry c occupa le celle da c*Ne a (c+1)*Ne - 1
		var vals []uint64
		for r := c * info.Ne; r < (c+1)*info.Ne; r++ {
			noised := ans.Get(r, 0) + offset
			vals = append(vals, p.Round(noised))
		}
		entries[c] = pir.ReconstructElem(vals, c*M+j, info)
	}

	ans.MatrixAdd(interm)
	return entries
}

func query_block(pi pir.SimplePIR, DB *pir.Database, A pir.State, serverState pir.State,
	H pir.Msg, p pir.Params, K uint64, M uint64, j uint64) string {

	s, query := pi.Query(j, A, p, DB.Info)

	var batch pir.MsgSlice
	batch.Data = append(batch.Data, query)
	answer := pi.Answer(DB, batch, serverState, A, p)

	entries := recover_column(p, DB.Info, H, query, answer, s, K, M, j)

	full := ""
	for _, value := range entries {
		full += num_to_text(value)
	}
	return strings.TrimRight(full, " ")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: tesi-pir <indice blocco> [soglia byte]")
		os.Exit(1)
	}
	index, err := strconv.ParseUint(os.Args[1], 10, 64)
	if err != nil {
		panic(err)
	}

	// soglia opzionale (default 32 KB)
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

	// DB e hint
	start := time.Now()
	DB, p, K, M := make_DB(pi, blocks)
	A := pi.Init(DB.Info, p)
	serverState, H := pi.Setup(DB, A, p)
	fmt.Fprintf(os.Stderr, "K=%d M=%d L=%d  setup: %v\n", K, M, p.L, time.Since(start))

	// client query
	start = time.Now()
	block := query_block(pi, DB, A, serverState, H, p, K, M, index)
	fmt.Fprintf(os.Stderr, "query: %v\n", time.Since(start))

	esito := "OK"
	if block != blocks[index] {
		esito = "ERRORE"
	}
	fmt.Printf("indice %d -> round %d, %d byte, %s\n", index, block_round(block), len(block), esito)
}
