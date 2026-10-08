package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"time"
	"text/tabwriter"

	"tesi-simplepir/utils"

	"github.com/ahenzinger/simplepir/pir"
)

// Parametri base
const (
	d         = 64
	SEC_PARAM = 1024
	LOGQ      = 32
	SOGLIA    = 24958 // Taglio outlier
	DIR       = "/home/giuper/Nocera/tesi-simplePIR-algo/BlocchiNoCert"
	WARMUP    = 2
	RIP       = 30
)

var VALORI_M = []int{70000}
var etichetta = "run4"

type risultato struct {
	cfg                  string
	M, K, L, P           uint64
	med, std, min        float64
	tput, setup          float64
	hintMB, upKB, downKB float64
}

// --- Funzioni statistiche base ---

func avg(d []float64) float64 {
	sum := 0.0
	for _, v := range d {
		sum += v
	}
	return sum / float64(len(d))
}

func stddev(d []float64) float64 {
	m := avg(d)
	sum := 0.0
	for _, v := range d {
		sum += math.Pow(v-m, 2)
	}
	return math.Sqrt(sum / float64(len(d)))
}

func mediana(d []float64) float64 {
	c := make([]float64, len(d))
	copy(c, d)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func minimo(d []float64) float64 {
	m := d[0]
	for _, v := range d {
		if v < m {
			m = v
		}
	}
	return m
}

// --- Preparazione dati ---

func read_blocks(dir string, maxLen int) [][]byte {
	var blocks [][]byte
	files, _ := os.ReadDir(dir)

	for _, f := range files {
		b, _ := os.ReadFile(dir + "/" + f.Name())
		if len(b) <= maxLen {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// Calcola quanti uint64 servono per il blocco più grande
func find_K(blocks [][]byte) uint64 {
	max := 0
	for _, b := range blocks {
		if len(b) > max {
			max = len(b)
		}
	}
	return uint64((max + 4 + 7) / 8) // +4 byte per la lunghezza
}

// Inserisce lunghezza + dati + padding
func pack(block []byte, K uint64) []uint64 {
	buf := make([]byte, K*8)
	binary.BigEndian.PutUint32(buf, uint32(len(block)))
	copy(buf[4:], block)

	out := make([]uint64, K)
	for i := range out {
		out[i] = binary.BigEndian.Uint64(buf[i*8:])
	}
	return out
}

// Rimuove il padding usando i primi 4 byte
func unpack(entries []uint64) []byte {
	buf := make([]byte, len(entries)*8)
	for i, val := range entries {
		binary.BigEndian.PutUint64(buf[i*8:], val)
	}
	n := binary.BigEndian.Uint32(buf)
	return buf[4 : 4+n]
}

// SimplePIR
func make_DB(pi pir.SimplePIR, blocks [][]byte) (*pir.Database, pir.Params, uint64) {
	M := uint64(len(blocks))
	K := find_K(blocks)

	p := pi.PickParamsGivenDimensions(1, M, SEC_PARAM, LOGQ)
	Ne := pir.Compute_num_entries_base_p(p.P, d)
	p = pi.PickParamsGivenDimensions(K*Ne, M, SEC_PARAM, LOGQ)

	vals := make([]uint64, K*M)
	for j, b := range blocks {
		for c, val := range pack(b, K) {
			vals[uint64(c)*M+uint64(j)] = val
		}
	}
	return pir.MakeDB(K*M, d, &p, vals), p, K
}

func recover_column(p pir.Params, info pir.DBinfo, H pir.Msg, query pir.Msg, answer pir.Msg, client pir.State, K uint64) []uint64 {
	secret := client.Data[0]
	ans := answer.Data[0]

	offset := uint64(0)
	for j := uint64(0); j < p.M; j++ {
		offset += (p.P / 2) * query.Data[0].Get(j, 0)
	}
	offset = (1 << p.Logq) - (offset % (1 << p.Logq))

	interm := pir.MatrixMul(H.Data[0], secret)
	ans.MatrixSub(interm)

	entries := make([]uint64, K)
	for c := uint64(0); c < K; c++ {
		var vals []uint64
		for r := c * info.Ne; r < (c+1)*info.Ne; r++ {
			vals = append(vals, p.Round(ans.Get(r, 0)+offset))
		}
		entries[c] = pir.ReconstructElem(vals, c, info)
	}
	ans.MatrixAdd(interm)
	return entries
}

// Benchmark

func testAlgorand(blocks [][]byte, M int) risultato {
	blocks = blocks[:M]
	fmt.Printf("\n=== Test Algorand: %d blocchi ===\n", M)

	pi := pir.SimplePIR{}
	DB, p, K := make_DB(pi, blocks)

	// Setup server
	start := time.Now()
	A := pi.Init(DB.Info, p)
	serverState, H := pi.Setup(DB, A, p)
	setup := time.Since(start).Seconds()

	// Controllo di correttezza su un blocco a caso
	idx := rand.Uint64N(uint64(M))
	cli, q := pi.Query(idx, A, p, DB.Info)
	ans := pi.Answer(DB, pir.MakeMsgSlice(q), serverState, A, p)
	ansblock := unpack(recover_column(p, DB.Info, H, q, ans, cli, K))

	if !bytes.Equal(ansblock, blocks[idx]) {
		panic("PIR fallito: il blocco non corrisponde!")
	}

	jsonStr, err := utils.DecodeMsgpackToJson(ansblock)
	if err != nil {
		panic(fmt.Sprintf("Decodifica fallita: %v", err))
	}

	fmt.Println(jsonStr)

	// Misurazione vera e propria
	debug.SetGCPercent(-1) // Disabilito GC per non falsare i tempi
	var tempi []float64

	for r := 0; r < WARMUP+RIP; r++ {
		j := rand.Uint64N(uint64(M))
		cli, q = pi.Query(j, A, p, DB.Info)
		batch := pir.MakeMsgSlice(q)

		runtime.GC()
		start = time.Now()
		pi.Answer(DB, batch, serverState, A, p)
		t := time.Since(start).Seconds()

		if r >= WARMUP {
			tempi = append(tempi, t)
		}
	}
	debug.SetGCPercent(100)

	cfg := fmt.Sprintf("Algorand M=%d", M)
	salva_tempi(cfg, tempi)

	// Ritorno le metriche aggregate
	med := mediana(tempi)
	return risultato{
		cfg: cfg,
		M:   p.M, K: K, L: p.L, P: p.P,
		med:    med * 1000,
		std:    stddev(tempi) * 1000,
		min:    minimo(tempi) * 1000,
		tput:   (math.Log2(float64(p.P)) * float64(p.L*p.M) / 8388608) / med,
		hintMB: float64(p.L*p.N*p.Logq) / 8388608,
		upKB:   float64(p.M*p.Logq) / 8192,
		downKB: float64(p.L*p.Logq) / 8192,
		setup:  setup,
	}
}

func salva_tempi(cfg string, tempi []float64) {
	f, _ := os.OpenFile("tempi.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	for i, t := range tempi {
		fmt.Fprintf(f, "%s,%s,%d,%.4f\n", etichetta, cfg, i, t*1000)
	}
}

func main() {
	if len(os.Args) > 1 {
		etichetta = os.Args[1]
	}

	blocks := read_blocks(DIR, SOGLIA)
	fmt.Printf("Blocchi caricati: %d\n", len(blocks))

	f, _ := os.OpenFile("risultati.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	w := tabwriter.NewWriter(f, 0, 4, 2, ' ', 0)

	fmt.Fprintf(w, "\n--- %s ---\n", etichetta)
	fmt.Fprintf(w, "Config\tM\tK\tL\tp\tms\tMB/s\thintMB\n")

	for _, M := range VALORI_M {
		if M > len(blocks) {
			continue
		}

		runtime.GC()
		r := testAlgorand(blocks, M)

		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%.2f\t%.0f\t%.2f\n",
			r.cfg, r.M, r.K, r.L, r.P, r.med, r.tput, r.hintMB)
	}
	w.Flush()
}
