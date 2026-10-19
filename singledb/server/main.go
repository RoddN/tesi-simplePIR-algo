package main

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"tesi-simplepir/blockpir"
	"tesi-simplepir/metrics"
	"tesi-simplepir/singledb"

	"github.com/ahenzinger/simplepir/pir"
)

var (
	pi          pir.SimplePIR
	DB          *pir.Database
	p           pir.Params
	A           pir.State
	serverState pir.State
)

func main() {
	debug.SetGCPercent(10)

	fmt.Println("[SERVER] Carico i blocchi")
	blocks, unique, err := blockpir.LoadRepeated(singledb.BlocksDir, singledb.MaxBlockBytes, singledb.NumBlocks, singledb.RepeatBlocks)
	if err != nil {
		fmt.Println("[SERVER] Errore:", err)
		os.Exit(1)
	}

	var rawBytes int64
	for _, b := range blocks {
		rawBytes += int64(len(b))
	}
	fmt.Printf("[SERVER] %d blocchi (%d unici), %.2f MB di dati\n", len(blocks), unique, float64(rawBytes)/(1<<20))

	startDB := time.Now()
	var K uint64
	DB, p, K = blockpir.BuildDB(pi, blocks)
	dbTime := time.Since(startDB)

	startHint := time.Now()
	A = pi.Init(DB.Info, p)
	var H pir.Msg

	serverState, H = pi.Setup(DB, A, p)
	hintTime := time.Since(startHint)

	fmt.Printf("[SERVER] DB in %v, hint in %v\n", dbTime, hintTime)

	hint := singledb.Hint{
		Params: p,
		Info:   DB.Info,
		A:      A,
		H:      H,
		K:      K,
		M:      len(blocks),
		Stats: metrics.SetupStats{
			UniqueBlocks: unique,
			RawBytes:     rawBytes,
			DBBytes:      int64(K) * 8 * int64(len(blocks)),
			DBTime:       dbTime,
			HintTime:     hintTime,
		},
	}
	if err := blockpir.SaveGob(singledb.HintFile, hint); err != nil {
		fmt.Println("[SERVER] Errore nel salvare l'hint:", err)
		os.Exit(1)
	}

	debug.FreeOSMemory()
	debug.SetGCPercent(-1)

	fmt.Printf("[SERVER] Pronto su %s\n", singledb.ListenAddr)
	http.HandleFunc("/query", handleQuery)
	if err := http.ListenAndServe(singledb.ListenAddr, nil); err != nil {
		fmt.Println("[SERVER] Errore:", err)
		os.Exit(1)
	}
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	var q pir.Msg
	if err := gob.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, "query non valida", http.StatusBadRequest)
		return
	}

	start := time.Now()

	ans := pi.Answer(DB, pir.MakeMsgSlice(q), serverState, A, p)
	resp := singledb.Response{Answer: ans, ServerTime: time.Since(start)}

	w.Header().Set("Content-Type", "application/octet-stream")
	gob.NewEncoder(w).Encode(resp)
}
