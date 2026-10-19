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
	"tesi-simplepir/dummyrow"

	"github.com/ahenzinger/simplepir/pir"
)

type Shard struct {
	DB    *pir.Database
	p     pir.Params
	A     pir.State
	state pir.State
}

var (
	pi     pir.SimplePIR
	shards []Shard
)

func main() {
	debug.SetGCPercent(10)

	fmt.Println("[SERVER] Carico i blocchi")
	blocks, _, err := blockpir.LoadBlocks(dummyrow.BlocksDir, dummyrow.MaxBlockBytes, dummyrow.NumBlocks)
	if err != nil {
		fail(err)
	}

	n := dummyrow.NumShards()
	shards = make([]Shard, n)
	fmt.Printf("[SERVER] %d blocchi, setup di %d shard in parallelo\n\n", len(blocks), n)

	var wg sync.WaitGroup
	errs := make([]error, n)
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = setupShard(i, blocks)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			fail(err)
		}
	}

	debug.FreeOSMemory()
	debug.SetGCPercent(-1)

	fmt.Printf("\n[SERVER] Pronto in %v su %s\n", time.Since(start), dummyrow.ListenAddr)
	http.HandleFunc("/query", handleQuery)
	if err := http.ListenAndServe(dummyrow.ListenAddr, nil); err != nil {
		fail(err)
	}
}

func setupShard(i int, blocks [][]byte) error {
	from := i * dummyrow.ShardSize
	to := min(from+dummyrow.ShardSize, len(blocks))
	fmt.Printf("[Shard %d] Setup DB (%d blocchi)\n", i, to-from)

	startDB := time.Now()
	DB, p, K := dummyrow.BuildShardDB(pi, blocks[from:to])
	dbTime := time.Since(startDB)

	startHint := time.Now()
	A := pi.Init(DB.Info, p)
	state, H := pi.Setup(DB, A, p)
	hintTime := time.Since(startHint)

	fmt.Printf("[Shard %d] DB in %v, hint in %v\n", i, dbTime, hintTime)

	hint := dummyrow.Hint{Params: p, Info: DB.Info, A: A, H: H, K: K, M: to - from}
	if err := blockpir.SaveGob(fmt.Sprintf(dummyrow.HintFilePattern, i), hint); err != nil {
		return err
	}

	shards[i] = Shard{DB: DB, p: p, A: A, state: state}
	return nil
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req dummyrow.ShardedQuery
	if err := gob.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Queries) != len(shards) {
		http.Error(w, "query non valida", http.StatusBadRequest)
		return
	}

	resp := dummyrow.ShardedResponse{Answers: make([]pir.Msg, len(shards))}
	for i, s := range shards {
		resp.Answers[i] = pi.Answer(s.DB, pir.MakeMsgSlice(req.Queries[i]), s.state, s.A, s.p)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	gob.NewEncoder(w).Encode(resp)
}

func fail(err error) {
	fmt.Println("[SERVER] Errore:", err)
	os.Exit(1)
}
