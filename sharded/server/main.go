package main

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"tesi-simplepir/blockpir"
	"tesi-simplepir/metrics"
	"tesi-simplepir/sharded"

	"github.com/ahenzinger/simplepir/pir"
)

var (
	pi     pir.SimplePIR
	p      pir.Params
	A      pir.State
	shards []*pir.Database
	states []pir.State
)

func main() {
	debug.SetGCPercent(10)

	fmt.Println("[SERVER] Carico i blocchi")
	blocks, unique, err := blockpir.LoadRepeated(sharded.BlocksDir, sharded.MaxBlockBytes, sharded.NumBlocks, sharded.RepeatBlocks)
	if err != nil {
		fail(err)
	}

	var rawBytes int64
	for _, b := range blocks {
		rawBytes += int64(len(b))
	}
	n, size := sharded.NumShards, sharded.ShardSize()
	fmt.Printf("[SERVER] %d blocchi (%d unici), %.2f MB di dati, %d shard da %d colonne\n",
		len(blocks), unique, float64(rawBytes)/(1<<20), n, size)

	K := blockpir.EntriesPerBlock(blocks)

	startDB := time.Now()
	shards = make([]*pir.Database, n)
	for j := range shards {
		shards[j], p = blockpir.BuildDBWithK(pi, shardBlocks(blocks, j, size), K, uint64(n*size))
	}
	dbTime := time.Since(startDB)

	startHint := time.Now()

	var seed pir.CompressedState
	A, seed = pi.InitCompressed(shards[0].Info, p)
	blockpir.ReseedPRG(pi)
	states = make([]pir.State, n)
	H := make([]pir.Msg, n)
	var wg sync.WaitGroup
	for j := range shards {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()

			states[j], H[j] = pi.Setup(shards[j], A, p)
		}(j)
	}
	wg.Wait()
	hintTime := time.Since(startHint)

	fmt.Printf("[SERVER] DB in %v, hint in %v\n", dbTime, hintTime)

	hint := sharded.Hint{
		Params:    p,
		Info:      shards[0].Info,
		Seed:      seed,
		H:         H,
		K:         K,
		ShardSize: size,
		NumBlocks: len(blocks),
		Stats: metrics.SetupStats{
			UniqueBlocks: unique,
			RawBytes:     rawBytes,
			DBBytes:      int64(K) * 8 * int64(n*size),
			DBTime:       dbTime,
			HintTime:     hintTime,
		},
	}
	if err := blockpir.SaveGob(sharded.HintFile, hint); err != nil {
		fail(err)
	}

	debug.FreeOSMemory()
	debug.SetGCPercent(-1)

	fmt.Printf("[SERVER] Pronto su %s\n", sharded.ListenAddr)
	http.HandleFunc("/query", handleQuery)
	if err := http.ListenAndServe(sharded.ListenAddr, nil); err != nil {
		fail(err)
	}
}

func shardBlocks(blocks [][]byte, j, size int) [][]byte {
	from := j * size
	to := min(from+size, len(blocks))

	part := make([][]byte, size)
	copy(part, blocks[from:to])
	for i := to - from; i < size; i++ {
		part[i] = []byte{}
	}
	return part
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req sharded.Request
	if err := gob.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Queries) != len(shards) {
		http.Error(w, "query non valida", http.StatusBadRequest)
		return
	}

	start := time.Now()

	answers := make([]pir.Msg, len(shards))
	if sharded.ParallelAnswers {
		var wg sync.WaitGroup
		for j := range shards {
			wg.Add(1)
			go func(j int) {
				defer wg.Done()
				answers[j] = pi.Answer(shards[j], pir.MakeMsgSlice(req.Queries[j]), states[j], A, p)
			}(j)
		}
		wg.Wait()
	} else {
		for j := range shards {
			answers[j] = pi.Answer(shards[j], pir.MakeMsgSlice(req.Queries[j]), states[j], A, p)
		}
	}
	for j := 1; j < len(answers); j++ {
		answers[0].Data[0].MatrixAdd(answers[j].Data[0])
	}
	resp := sharded.Response{Answer: answers[0], ServerTime: time.Since(start)}

	w.Header().Set("Content-Type", "application/octet-stream")
	gob.NewEncoder(w).Encode(resp)
}

func fail(err error) {
	fmt.Println("[SERVER] Errore:", err)
	os.Exit(1)
}
