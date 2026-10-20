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
	"tesi-simplepir/singledb"

	"github.com/ahenzinger/simplepir/pir"
)

func main() {
	label := "run"
	if len(os.Args) > 1 {
		label = os.Args[1]
	}

	var hint singledb.Hint
	if err := blockpir.LoadGob(singledb.HintFile, &hint); err != nil {
		fail("hint non trovato, avviare prima il server: %v", err)
	}
	if hint.M != singledb.NumBlocks {
		fail("l'hint ha M = %d ma config.go dice NumBlocks = %d: riavviare il server", hint.M, singledb.NumBlocks)
	}

	blocks, _, err := blockpir.LoadRepeated(singledb.BlocksDir, singledb.MaxBlockBytes, singledb.NumBlocks, singledb.RepeatBlocks)
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
	total := singledb.WarmupRuns + singledb.MeasuredRuns
	var runs []metrics.Run

	for r := 0; r < total; r++ {
		runtime.GC()
		run, err := doQuery(pi, hint, A, blocks)
		if err != nil {
			fail("run %d: %v", r+1, err)
		}
		fmt.Printf("[CLIENT] run %d/%d: query %.2f ms, server %.2f ms, RTT %.2f ms\n",
			r+1, total, run.Query.Seconds()*1000, run.Server.Seconds()*1000, run.RTT.Seconds()*1000)

		if r >= singledb.WarmupRuns {
			runs = append(runs, run)
		}
	}

	p := hint.Params
	summary := metrics.Summary{
		Label:      label,
		M:          hint.M,
		Shards:     1,
		HBytes:     int64(p.L * p.N * p.Logq / 8),
		ABytes:     int64(len(hint.Seed.Seed)),
		ATime:      aTime,
		WarmupRuns: singledb.WarmupRuns,
		Setup:      hint.Stats,
	}
	if err := metrics.Save(singledb.ResultsDir, summary, runs); err != nil {
		fail("salvataggio dei risultati: %v", err)
	}
	fmt.Printf("[CLIENT] Risultati salvati in %s/\n", singledb.ResultsDir)
}

func doQuery(pi pir.SimplePIR, hint singledb.Hint, A pir.State, blocks [][]byte) (metrics.Run, error) {
	idx := rand.Uint64N(uint64(hint.M))
	run := metrics.Run{Block: idx}

	start := time.Now()
	secret, q := pi.Query(idx, A, hint.Params, hint.Info)
	run.Query = time.Since(start)

	var body bytes.Buffer
	if err := gob.NewEncoder(&body).Encode(q); err != nil {
		return run, err
	}
	run.UploadBytes = body.Len()

	start = time.Now()
	httpResp, err := http.Post(singledb.ServerURL, "application/octet-stream", &body)
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

	var resp singledb.Response
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&resp); err != nil {
		return run, err
	}
	run.Server = resp.ServerTime

	start = time.Now()
	block := blockpir.DecodeBlock(blockpir.DecodeAnswer(hint.Params, hint.Info, hint.H, q, resp.Answer, secret, hint.K))
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
