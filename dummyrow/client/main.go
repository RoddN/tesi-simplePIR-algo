package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	"tesi-simplepir/blockpir"
	"tesi-simplepir/dummyrow"

	"github.com/ahenzinger/simplepir/pir"
)

func main() {
	fmt.Println("[CLIENT] Avvio")

	n := dummyrow.NumShards()
	hints := make([]dummyrow.Hint, n)
	for i := range hints {
		if err := blockpir.LoadGob(fmt.Sprintf(dummyrow.HintFilePattern, i), &hints[i]); err != nil {
			fail("hint dello shard %d non trovato, avviare prima il server: %v", i, err)
		}
	}

	blocks, names, err := blockpir.LoadBlocks(dummyrow.BlocksDir, dummyrow.MaxBlockBytes, dummyrow.NumBlocks)
	if err != nil {
		fail("%v", err)
	}

	pi := pir.SimplePIR{}

	fmt.Println("[CLIENT] Preparo le query fittizie")
	start := time.Now()
	queries := make([]pir.Msg, n)
	for i, h := range hints {
		_, queries[i] = pi.Query(0, h.A, h.Params, h.Info)
	}
	fmt.Printf("[CLIENT] Query fittizie pronte in %v\n\n", time.Since(start))

	idx := rand.IntN(len(blocks))
	shard := idx / dummyrow.ShardSize
	col := uint64(idx%dummyrow.ShardSize) + 1
	h := hints[shard]
	fmt.Printf("[CLIENT] Blocco %d (%s) -> shard %d, colonna %d\n", idx, names[idx], shard, col)

	start = time.Now()
	secret, realQuery := pi.Query(col, h.A, h.Params, h.Info)
	queryTime := time.Since(start)

	queries[shard] = realQuery

	var body bytes.Buffer
	if err := gob.NewEncoder(&body).Encode(dummyrow.ShardedQuery{Queries: queries}); err != nil {
		fail("%v", err)
	}

	fmt.Println("--- Metriche ---")
	fmt.Printf("Generazione query: %v\n", queryTime)
	fmt.Printf("Upload (%d query):  %.2f KB\n", n, float64(body.Len())/1024)

	start = time.Now()
	httpResp, err := http.Post(dummyrow.ServerURL, "application/octet-stream", &body)
	if err != nil {
		fail("server non raggiungibile: %v", err)
	}
	raw, err := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	rtt := time.Since(start)
	if err != nil || httpResp.StatusCode != http.StatusOK {
		fail("risposta non valida dal server (%s): %v", httpResp.Status, err)
	}
	fmt.Printf("Query time (RTT):  %v\n\n", rtt)

	var resp dummyrow.ShardedResponse
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&resp); err != nil {
		fail("%v", err)
	}

	block := blockpir.DecodeBlock(blockpir.DecodeAnswer(h.Params, h.Info, h.H, realQuery, resp.Answers[shard], secret, h.K))
	if !bytes.Equal(block, blocks[idx]) {
		fail("blocco diverso dall'originale")
	}

	text, _ := blockpir.MsgpackToJSON(block)
	fmt.Println("[CLIENT] Blocco recuperato")
	if len(text) > 250 {
		fmt.Printf("Estratto: %s...\n", text[50:250])
	} else {
		fmt.Println("Estratto:", text)
	}
}

func fail(format string, args ...any) {
	fmt.Printf("[CLIENT] Errore: "+format+"\n", args...)
	os.Exit(1)
}
