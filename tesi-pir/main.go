package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"time"

	"github.com/ahenzinger/simplepir/pir"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	d         = uint64(64) // bit di ogni entry del DB
	SEC_PARAM = 1024       // dimensione n del segreto LWE
	LOGQ      = 32         // log del modulo q

	// soglia sugli outlier: Q3 + 1.5*IQR calcolato con summary.py
	SOGLIA = 45789

	DIR          = "../blocchi_senza_cert"
	WARMUP       = 2  // primi giri che non conto
	RIP          = 30 // giri misurati
	NUM_VERIFICA = 20 // blocchi a caso da controllare prima di misurare
)

// quanti blocchi mettere nel DB, es. {10000, 20000, 40000}
var VALORI_M = []int{10000}

// nome dell'esecuzione, si puo' passare da riga di comando: go run . run2
var etichetta = "run1"

type risultato struct {
	cfg                          string
	M, K, L, P                   uint64
	tempoMed, tempoStd, tempoMin float64 // ms
	tput, tputUtile              float64 // MB/s
	hintMB, upKB, downKB         float64
	setup                        float64 // s
}

func avg(data []float64) float64 {
	sum := 0.0
	for _, elem := range data {
		sum += elem
	}
	return sum / float64(len(data))
}

func stddev(data []float64) float64 {
	m := avg(data)
	sum := 0.0
	for _, elem := range data {
		sum += math.Pow(elem-m, 2)
	}
	return math.Sqrt(sum / float64(len(data)))
}

func mediana(data []float64) float64 {
	c := make([]float64, len(data))
	copy(c, data)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func minimo(data []float64) float64 {
	m := data[0]
	for _, v := range data {
		if v < m {
			m = v
		}
	}
	return m
}

// read_blocks legge i file della cartella e scarta quelli piu' grandi della soglia
func read_blocks(dir string, threshold int) [][]byte {
	var blocks [][]byte

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
		blocks = append(blocks, content)
	}
	return blocks
}

// find_K dice quante entry da 8 byte servono per il blocco piu' grande.
// I 4 byte in piu' servono a salvare la lunghezza del blocco.
func find_K(blocks [][]byte) uint64 {
	max_len := 0
	for _, block := range blocks {
		if len(block) > max_len {
			max_len = len(block)
		}
	}
	return uint64((max_len + 4 + 7) / 8)
}

// pack trasforma un blocco in K numeri da 64 bit:
// lunghezza nei primi 4 byte, poi il blocco, poi zeri fino a K*8 byte.
func pack(block []byte, K uint64) []uint64 {
	buf := make([]byte, K*8)
	binary.BigEndian.PutUint32(buf, uint32(len(block)))
	copy(buf[4:], block)

	entries := make([]uint64, K)
	for c := range entries {
		entries[c] = binary.BigEndian.Uint64(buf[c*8:])
	}
	return entries
}

// unpack fa il contrario di pack e usa la lunghezza per togliere gli zeri finali
func unpack(entries []uint64) []byte {
	buf := make([]byte, len(entries)*8)
	for c, value := range entries {
		binary.BigEndian.PutUint64(buf[c*8:], value)
	}
	n := binary.BigEndian.Uint32(buf)
	if int(n) > len(buf)-4 {
		panic("lunghezza del blocco non valida")
	}
	return buf[4 : 4+n]
}

// block_round legge il numero di round dal blocco msgpack
func block_round(block []byte) uint64 {
	var b struct {
		Block struct {
			Rnd uint64 `msgpack:"rnd"`
		} `msgpack:"block"`
	}
	if err := msgpack.Unmarshal(block, &b); err != nil {
		panic(err)
	}
	return b.Block.Rnd
}

// make_DB mette ogni blocco in una colonna della matrice.
// La entry c del blocco j va in posizione c*M + j: MakeDB la mette
// nella riga c (occupando Ne celle) e nella colonna j.
func make_DB(pi pir.SimplePIR, blocks [][]byte) (*pir.Database, pir.Params, uint64, uint64) {
	M := uint64(len(blocks))
	K := find_K(blocks)

	// p dipende solo da M: prima lo scelgo, poi calcolo Ne e l'altezza L = K*Ne
	p := pi.PickParamsGivenDimensions(1, M, SEC_PARAM, LOGQ)
	Ne := pir.Compute_num_entries_base_p(p.P, d)
	p = pi.PickParamsGivenDimensions(K*Ne, M, SEC_PARAM, LOGQ)

	vals := make([]uint64, K*M)
	for j, block := range blocks {
		for c, value := range pack(block, K) {
			vals[uint64(c)*M+uint64(j)] = value
		}
	}

	DB := pir.MakeDB(K*M, d, &p, vals)
	return DB, p, K, M
}

// recover_column fa gli stessi passaggi di pi.Recover (offset, sottrazione
// di H*s, arrotondamento) ma su tutta la colonna ricevuta, e restituisce
// le K entry del blocco invece di una sola.
func recover_column(p pir.Params, info pir.DBinfo, H pir.Msg, query pir.Msg,
	answer pir.Msg, client pir.State, K uint64) []uint64 {

	secret := client.Data[0]
	ans := answer.Data[0]

	ratio := p.P / 2
	offset := uint64(0)
	for j := uint64(0); j < p.M; j++ {
		offset += ratio * query.Data[0].Get(j, 0)
	}
	offset %= (1 << p.Logq)
	offset = (1 << p.Logq) - offset

	interm := pir.MatrixMul(H.Data[0], secret)
	ans.MatrixSub(interm)

	entries := make([]uint64, K)
	for c := uint64(0); c < K; c++ {
		var vals []uint64
		for r := c * info.Ne; r < (c+1)*info.Ne; r++ {
			noised := ans.Get(r, 0) + offset
			vals = append(vals, p.Round(noised))
		}
		entries[c] = pir.ReconstructElem(vals, c, info)
	}
	ans.MatrixAdd(interm)

	return entries
}

// query_block chiede la colonna j al server e ricostruisce il blocco
func query_block(pi pir.SimplePIR, DB *pir.Database, A pir.State, serverState pir.State,
	H pir.Msg, p pir.Params, K uint64, j uint64) []byte {

	client, query := pi.Query(j, A, p, DB.Info)
	answer := pi.Answer(DB, pir.MakeMsgSlice(query), serverState, A, p)
	return unpack(recover_column(p, DB.Info, H, query, answer, client, K))
}

// salva_tempi aggiunge i tempi in fondo a tempi.csv
// formato: etichetta,configurazione,giro,tempo_ms
func salva_tempi(cfg string, tempi []float64) {
	f, err := os.OpenFile("tempi.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	for i, t := range tempi {
		fmt.Fprintf(f, "%s,%s,%d,%.4f\n", etichetta, cfg, i, t*1000)
	}
}

// calcola mette insieme i numeri per la tabella finale.
// Il throughput e' calcolato come nel repo: dimensione del DB / tempo di Answer.
func calcola(cfg string, p pir.Params, K uint64, tempi []float64, utiliMB float64, setup float64) risultato {
	dimMB := math.Log2(float64(p.P)) * float64(p.L*p.M) / (8 * 1024 * 1024)
	med := mediana(tempi)

	return risultato{
		cfg:       cfg,
		M:         p.M,
		K:         K,
		L:         p.L,
		P:         p.P,
		tempoMed:  med * 1000,
		tempoStd:  stddev(tempi) * 1000,
		tempoMin:  minimo(tempi) * 1000,
		tput:      dimMB / med,
		tputUtile: utiliMB / med,
		hintMB:    float64(p.L*p.N*p.Logq) / 8 / 1024 / 1024,
		upKB:      float64(p.M*p.Logq) / 8 / 1024,
		downKB:    float64(p.L*p.Logq) / 8 / 1024,
		setup:     setup,
	}
}

// testAlgorand costruisce il DB con i primi M blocchi, fa il setup una volta,
// controlla che il PIR restituisca i blocchi giusti e misura il tempo di Answer.
func testAlgorand(blocks [][]byte, M int) risultato {
	blocks = blocks[:M]
	fmt.Printf("\n--- Test Algorand con %d blocchi ---\n", M)

	pi := pir.SimplePIR{}
	DB, p, K, _ := make_DB(pi, blocks)

	utili := 0
	for _, b := range blocks {
		utili += len(b)
	}
	fmt.Printf("K = %d, matrice %d x %d, p = %d\n", K, p.L, p.M, p.P)
	fmt.Printf("byte utili / byte con padding = %.2f\n", float64(utili)/float64(K*8*uint64(M)))

	// setup vero, una volta sola: calcola l'hint e poi comprime il DB.
	// Da qui in poi il DB resta compresso, che e' quello che serve ad Answer.
	start := time.Now()
	A := pi.Init(DB.Info, p)
	serverState, H := pi.Setup(DB, A, p)
	setup := time.Since(start).Seconds()
	fmt.Printf("setup: %.2f s\n", setup)

	// prima di misurare controllo primo, ultimo e qualche blocco a caso
	da_controllare := []uint64{0, uint64(M - 1)}
	for i := 0; i < NUM_VERIFICA; i++ {
		da_controllare = append(da_controllare, rand.Uint64N(uint64(M)))
	}
	for _, j := range da_controllare {
		block := query_block(pi, DB, A, serverState, H, p, K, j)
		if !bytes.Equal(block, blocks[j]) {
			panic(fmt.Sprintf("il blocco %d recuperato e' diverso dall'originale", j))
		}
	}
	fmt.Printf("controllati %d blocchi: OK (round da %d a %d)\n",
		len(da_controllare), block_round(blocks[0]), block_round(blocks[M-1]))

	// misura: cronometro solo Answer, la query la preparo prima.
	// Tolgo il garbage collector perche' altrimenti puo' partire durante la misura.
	debug.SetGCPercent(-1)
	var tempi []float64
	for r := 0; r < WARMUP+RIP; r++ {
		j := rand.Uint64N(uint64(M))
		client, query := pi.Query(j, A, p, DB.Info)
		batch := pir.MakeMsgSlice(query)

		runtime.GC()
		start = time.Now()
		answer := pi.Answer(DB, batch, serverState, A, p)
		t := time.Since(start).Seconds()

		// controllo anche qui che il blocco ricevuto sia quello giusto
		block := unpack(recover_column(p, DB.Info, H, query, answer, client, K))
		if !bytes.Equal(block, blocks[j]) {
			panic(fmt.Sprintf("blocco %d sbagliato durante la misura", j))
		}

		if r >= WARMUP {
			tempi = append(tempi, t)
		}
	}
	debug.SetGCPercent(100)

	cfg := fmt.Sprintf("Algorand M=%d", M)
	salva_tempi(cfg, tempi)
	return calcola(cfg, p, K, tempi, float64(utili)/1024/1024, setup)
}

// testQuadrato serve come confronto: stesso numero di entry da 64 bit,
// ma dati casuali e matrice quasi quadrata come nei test del repo.
// Qui non controllo i risultati, quindi l'hint non serve e uso FakeSetup.
func testQuadrato(N uint64) risultato {
	fmt.Printf("\n--- Test quadrato casuale con %d entry ---\n", N)

	pi := pir.SimplePIR{}
	p := pi.PickParams(N, d, SEC_PARAM, LOGQ)
	DB := pir.MakeRandomDB(N, d, &p)

	A := pi.Init(DB.Info, p)
	serverState, _ := pi.FakeSetup(DB, p) // comprime il DB senza calcolare l'hint

	debug.SetGCPercent(-1)
	var tempi []float64
	for r := 0; r < WARMUP+RIP; r++ {
		j := rand.Uint64N(p.M)
		_, query := pi.Query(j, A, p, DB.Info)
		batch := pir.MakeMsgSlice(query)

		runtime.GC()
		start := time.Now()
		pi.Answer(DB, batch, serverState, A, p)
		t := time.Since(start).Seconds()

		if r >= WARMUP {
			tempi = append(tempi, t)
		}
	}
	debug.SetGCPercent(100)

	cfg := fmt.Sprintf("Quadrato N=%d", N)
	salva_tempi(cfg, tempi)
	// dati casuali, quindi niente padding: i byte utili sono N*8
	return calcola(cfg, p, 0, tempi, float64(N*8)/1024/1024, 0)
}

func main() {
	if len(os.Args) > 1 {
		etichetta = os.Args[1]
	}

	fmt.Println("Avvio benchmark...")
	blocks := read_blocks(DIR, SOGLIA)
	fmt.Printf("%d blocchi sotto la soglia\n", len(blocks))

	var risultati []risultato
	for _, M := range VALORI_M {
		if M > len(blocks) {
			fmt.Printf("salto M=%d, ci sono solo %d blocchi\n", M, len(blocks))
			continue
		}

		runtime.GC()
		r := testAlgorand(blocks, M)
		risultati = append(risultati, r)

		runtime.GC()
		risultati = append(risultati, testQuadrato(r.K*uint64(M)))
	}

	// la tabella va sia a schermo sia in fondo a risultati.txt
	f, err := os.OpenFile("risultati.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	out := io.MultiWriter(os.Stdout, f)

	fmt.Fprintf(out, "\n%s - %s (warmup %d, giri %d)\n", etichetta, time.Now().Format("2006-01-02 15:04"), WARMUP, RIP)
	fmt.Fprintf(out, "%-22s %6s %6s %5s %10s %7s %7s %10s %10s %8s %8s %8s %7s\n",
		"configurazione", "M", "L", "p", "answer ms", "std", "min",
		"MB/s", "utile MB/s", "hint MB", "query KB", "risp KB", "setup s")

	// valori dal README (AWS c5n.metal, DB da 1 GB). 100 ms = 1 GB / 10 GB/s,
	// 242 KB e' la comunicazione online totale (query + risposta)
	fmt.Fprintf(out, "%-22s %6s %6s %5s %10s %7s %7s %10s %10s %8s %17s %7s\n",
		"paper (README)", "-", "-", "-", "~100", "-", "-", "~10240", "-", "121", "242 tot", "-")

	for _, r := range risultati {
		fmt.Fprintf(out, "%-22s %6d %6d %5d %10.3f %7.3f %7.3f %10.0f %10.0f %8.1f %8.1f %8.1f %7.1f\n",
			r.cfg, r.M, r.L, r.P, r.tempoMed, r.tempoStd, r.tempoMin,
			r.tput, r.tputUtile, r.hintMB, r.upKB, r.downKB, r.setup)
	}
}
