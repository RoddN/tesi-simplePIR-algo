package main

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"text/tabwriter"
	"time"

	"tesi-simplepir/blockpir"

	"github.com/ahenzinger/simplepir/pir"
)

const (
	blocksDir     = "../BlocchiNoCert"
	maxBlockBytes = 24958
	warmupRuns    = 2
	measuredRuns  = 30
	resultsDir    = "results"
)

var mValues = []int{150000}

var label = "run"

type result struct {
	M, K, L, P           uint64
	med, std, min        float64
	tput                 float64
	hintMB, upKB, downKB float64
	dbSizeGB             float64
	largeBlockKB         float64
	avgBlockKB           float64
	setupDBTime          float64
	setupHintTime        float64
}

func mean(d []float64) float64 {
	sum := 0.0
	for _, v := range d {
		sum += v
	}
	return sum / float64(len(d))
}

func stddev(d []float64) float64 {
	m := mean(d)
	sum := 0.0
	for _, v := range d {
		sum += math.Pow(v-m, 2)
	}
	return math.Sqrt(sum / float64(len(d)))
}

func median(d []float64) float64 {
	c := make([]float64, len(d))
	copy(c, d)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func minimum(d []float64) float64 {
	m := d[0]
	for _, v := range d {
		if v < m {
			m = v
		}
	}
	return m
}

func runBenchmark(blocks [][]byte, rounds []string, M int) result {
	blocks, rounds = blockpir.RepeatBlocks(blocks, rounds, M)
	fmt.Printf("\n=== Test Algorand: %d blocchi ===\n", M)

	var totalBytes int
	for _, b := range blocks {
		totalBytes += len(b)
	}
	avgBlockKB := (float64(totalBytes) / float64(len(blocks))) / 1024.0

	pi := pir.SimplePIR{}

	startDB := time.Now()
	DB, p, K := blockpir.BuildDB(pi, blocks)
	setupDBTime := time.Since(startDB).Seconds()

	startHint := time.Now()
	A := pi.Init(DB.Info, p)
	serverState, H := pi.Setup(DB, A, p)
	setupHintTime := time.Since(startHint).Seconds()

	idx := rand.Uint64N(uint64(M))
	secret, q := pi.Query(idx, A, p, DB.Info)
	ans := pi.Answer(DB, pir.MakeMsgSlice(q), serverState, A, p)
	block := blockpir.DecodeBlock(blockpir.DecodeAnswer(p, DB.Info, H, q, ans, secret, K))

	if !bytes.Equal(block, blocks[idx]) {
		panic("PIR fallito: blocco diverso dall'originale")
	}
	text, err := blockpir.MsgpackToJSON(block)
	if err != nil {
		panic(fmt.Sprintf("Decodifica fallita: %v", err))
	}
	fmt.Printf("Verifica ok, round %s: %s...\n", rounds[idx], text[:50])

	debug.SetGCPercent(-1)
	var times []float64

	for r := 0; r < warmupRuns+measuredRuns; r++ {
		j := rand.Uint64N(uint64(M))
		_, q = pi.Query(j, A, p, DB.Info)
		batch := pir.MakeMsgSlice(q)

		runtime.GC()
		start := time.Now()
		pi.Answer(DB, batch, serverState, A, p)
		t := time.Since(start).Seconds()

		if r >= warmupRuns {
			times = append(times, t)
		}
	}
	debug.SetGCPercent(100)

	saveTimes(fmt.Sprintf("Algorand M=%d", M), times)

	med := median(times)
	return result{
		M: p.M, K: K, L: p.L, P: p.P,
		med:           med * 1000,
		std:           stddev(times) * 1000,
		min:           minimum(times) * 1000,
		tput:          (math.Log2(float64(p.P)) * float64(p.L*p.M) / 8388608) / med,
		hintMB:        float64(p.L*p.N*p.Logq) / 8388608,
		upKB:          float64(p.M*p.Logq) / 8192,
		downKB:        float64(p.L*p.Logq) / 8192,
		dbSizeGB:      float64(K*uint64(len(blocks))*8) / (1024 * 1024 * 1024),
		largeBlockKB:  float64(K*8) / 1024.0,
		avgBlockKB:    avgBlockKB,
		setupDBTime:   setupDBTime,
		setupHintTime: setupHintTime,
	}
}

func saveTimes(cfg string, times []float64) {
	f, err := os.OpenFile(filepath.Join(resultsDir, "times.csv"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	for i, t := range times {
		fmt.Fprintf(f, "%s,%s,%d,%.4f\n", label, cfg, i, t*1000)
	}
}

func main() {
	if len(os.Args) > 1 {
		label = os.Args[1]
	}

	blocks, rounds, err := blockpir.LoadBlocks(blocksDir, maxBlockBytes, 0)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Blocchi caricati: %d\n", len(blocks))

	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		panic(err)
	}
	f, err := os.OpenFile(filepath.Join(resultsDir, "summary.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := tabwriter.NewWriter(f, 0, 4, 2, ' ', 0)

	fmt.Fprintf(w, "\n--- Esecuzione Benchmark: %s ---\n", label)
	fmt.Fprintf(w, "M\tDB Size\tQuery Size\tQuery Time\tHint Size\tBlock Size\tLarge Block\tTime DB\tTime Hint\n")

	for _, M := range mValues {
		runtime.GC()
		r := runBenchmark(blocks, rounds, M)

		fmt.Fprintf(w, "%d\t%.2f GB\t%.2f KB\t%.2f ms\t%.2f MB\t%.2f KB\t%.2f KB\t%.2f s\t%.2f s\n",
			r.M, r.dbSizeGB, r.upKB, r.med, r.hintMB, r.avgBlockKB, r.largeBlockKB, r.setupDBTime, r.setupHintTime)
	}
	w.Flush()
	fmt.Printf("\nFatto, risultati in %s/\n", resultsDir)
}
