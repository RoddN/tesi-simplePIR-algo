package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"tesi-simplepir/blockpir"
	"tesi-simplepir/metrics"
	"tesi-simplepir/sharded"

	"github.com/ahenzinger/simplepir/pir"
)

func main() {
	label := "run"
	if len(os.Args) > 1 {
		label = os.Args[1]
	}

	var hint sharded.Hint
	if err := blockpir.LoadGob(sharded.HintFile, &hint); err != nil {
		fail("hint non trovato, avviare prima il server: %v", err)
	}
	if hint.NumBlocks != sharded.NumBlocks || len(hint.H) != sharded.NumShards {
		fail("l'hint è per %d blocchi e %d shard, config.go per %d e %d: riavviare il server",
			hint.NumBlocks, len(hint.H), sharded.NumBlocks, sharded.NumShards)
	}

	blocks, _, err := blockpir.LoadRepeated(sharded.BlocksDir, sharded.MaxBlockBytes, sharded.NumBlocks, sharded.RepeatBlocks)
	if err != nil {
		fail("%v", err)
	}

	pi := pir.SimplePIR{}

	start := time.Now()
	A := pi.DecompressState(hint.Info, hint.Params, hint.Seed)
	aTime := time.Since(start)
	blockpir.ReseedPRG(pi)
	fmt.Printf("[CLIENT] A generata dal seme in %v\n", aTime)

	debug.SetGCPercent(-1)
	total := sharded.WarmupRuns + sharded.MeasuredRuns
	var runs []metrics.Run

	for r := 0; r < total; r++ {
		runtime.GC()
		run, err := doQuery(pi, hint, A, blocks)
		if err != nil {
			fail("run %d: %v", r+1, err)
		}
		fmt.Printf("[CLIENT] run %d/%d: query %.2f ms, server %.2f ms, RTT %.2f ms\n",
			r+1, total, run.Query.Seconds()*1000, run.Server.Seconds()*1000, run.RTT.Seconds()*1000)

		if r >= sharded.WarmupRuns {
			runs = append(runs, run)
		}
	}

	p := hint.Params
	summary := metrics.Summary{
		Label:      label,
		M:          hint.NumBlocks,
		Shards:     len(hint.H),
		HBytes:     int64(len(hint.H)) * int64(p.L*p.N*p.Logq/8),
		ABytes:     int64(len(hint.Seed.Seed)),
		ATime:      aTime,
		WarmupRuns: sharded.WarmupRuns,
		Setup:      hint.Stats,
	}
	if err := metrics.Save(sharded.ResultsDir, summary, runs); err != nil {
		fail("salvataggio dei risultati: %v", err)
	}
	fmt.Printf("[CLIENT] Risultati salvati in %s/\n", sharded.ResultsDir)
}

func doQuery(pi pir.SimplePIR, hint sharded.Hint, A pir.State, blocks [][]byte) (metrics.Run, error) {
	idx := rand.IntN(len(blocks))

	shard := idx / hint.ShardSize
	col := uint64(idx % hint.ShardSize)
	run := metrics.Run{Block: uint64(idx)}

	start := time.Now()

	secret, q := pi.Query(col, A, hint.Params, hint.Info)
	run.Query = time.Since(start)

	var body bytes.Buffer
	if err := gob.NewEncoder(&body).Encode(q); err != nil {
		return run, err
	}
	run.UploadBytes = body.Len()

	start = time.Now()
	httpResp, err := http.Post(sharded.ServerURL, "application/octet-stream", &body)
	if err != nil {
		return run, fmt.Errorf("server non raggiungibile: %v", err)
	}

	raw, err := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	run.RTT = time.Since(start)
	if err != nil {
		return run, err
	}
	if httpResp.StatusCode != http.StatusOK {
		return run, fmt.Errorf("il server ha risposto %s", httpResp.Status)
	}
	run.DownloadBytes = len(raw)

	var resp sharded.Response
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&resp); err != nil {
		return run, err
	}
	run.Server = resp.ServerTime

	start = time.Now()

	block := blockpir.DecodeBlock(blockpir.DecodeAnswer(hint.Params, hint.Info, hint.H[shard], q, resp.Answers[shard], secret, hint.K))
	run.Decode = time.Since(start)

	if !bytes.Equal(block, blocks[idx]) {
		return run, fmt.Errorf("il blocco %d recuperato è diverso dall'originale", idx)
	}
	return run, nil
}

func fail(format string, args ...any) {
	fmt.Printf("[CLIENT] Errore: "+format+"\n", args...)
	os.Exit(1)
}
