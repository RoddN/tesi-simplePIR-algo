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
)

// risultato raccoglie i numeri di una configurazione per la tabella finale
type risultato struct {
	cfg           string
	tput, tputStd float64 // MB/s
	tempo         float64 // ms per Answer
	hintMB        float64
	upKB, downKB  float64
}

func avg(data []float64) float64 {
	sum := 0.0
	num := 0.0
	for _, elem := range data {
		sum += elem
		num += 1.0
	}
	if num == 0 {
		return 0
	}
	return sum / num
}

func stddev(data []float64) float64 {
	m := avg(data)
	sum := 0.0
	num := 0.0
	for _, elem := range data {
		sum += math.Pow(elem-m, 2)
		num += 1.0
	}
	if num == 0 {
		return 0
	}
	variance := sum / num
	return math.Sqrt(variance)
}

// calcola ricava tempo di Answer e dimensioni dei messaggi dai parametri.
// Attenzione: p.N e' la dimensione LWE (1024), non il numero di blocchi.
func calcola(cfg string, p pir.Params, tputs []float64) risultato {
	dimMB := math.Log2(float64(p.P)) * float64(p.L*p.M) / (8 * 1024 * 1024)
	tput := avg(tputs)

	return risultato{
		cfg:     cfg,
		tput:    tput,
		tputStd: stddev(tputs),
		tempo:   dimMB / tput * 1000,
		hintMB:  float64(p.L*p.N*p.Logq) / 8 / 1024 / 1024,
		upKB:    float64(p.M*p.Logq) / 8 / 1024,
		downKB:  float64(p.L*p.Logq) / 8 / 1024,
	}
}

// misura ripete la fase online nRip volte, come il benchmark degli autori.
// RunFakePIR salta il calcolo dell'hint, che non serve per il throughput.
func misura(pi pir.SimplePIR, DB *pir.Database, p pir.Params, idx uint64, nRip int) []float64 {
	var tputs []float64

	for j := 0; j < nRip; j++ {
		tput, _, _, _ := pir.RunFakePIR(&pi, DB, p, []uint64{idx}, nil, false)
		tputs = append(tputs, tput)
	}
	return tputs
}

func genera(N uint64) []uint64 {
	blocchi := make([]uint64, N)
	for i := range blocchi {
		blocchi[i] = rand.Uint64()
	}
	return blocchi
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

	var batch pir.MsgSlice
	batch.Data = append(batch.Data, query)
	answer := pi.Answer(DB, batch, serverState, A, p)

	entries := recover_column(p, DB.Info, H, query, answer, client, K)
	return unpack(entries)
}

// testAlgorand costruisce il DB con i blocchi veri, controlla che il PIR
// restituisca i blocchi giusti e poi misura le prestazioni.
func testAlgorand(dir string, threshold int, nRip int) risultato {
	fmt.Printf("\n--- Test Algorand su %s (soglia %d byte) ---\n", dir, threshold)

	blocks := read_blocks(dir, threshold)
	if len(blocks) == 0 {
		panic("nessun blocco sotto la soglia")
	}

	pi := pir.SimplePIR{}
	DB, p, K, M := make_DB(pi, blocks)
	fmt.Printf("blocchi: %d, K = %d entry per blocco, matrice %d x %d\n", M, K, p.L, p.M)

	// setup vero: il server calcola l'hint H = DB * A
	start := time.Now()
	A := pi.Init(DB.Info, p)
	serverState, H := pi.Setup(DB, A, p)
	fmt.Printf("setup (hint): %v\n", time.Since(start))

	// controllo che il blocco uscito dal PIR sia uguale all'originale
	for _, j := range []uint64{0, M / 2, M - 1} {
		start = time.Now()
		block := query_block(pi, DB, A, serverState, H, p, K, j)
		tempo := time.Since(start)

		if !bytes.Equal(block, blocks[j]) {
			panic(fmt.Sprintf("il blocco %d recuperato e' diverso dall'originale", j))
		}
		fmt.Printf("blocco %d: round %d, %d byte, OK (%v)\n", j, block_round(block), len(block), tempo)
	}

	// Setup ha compresso il DB in memoria: lo riporto allo stato iniziale,
	// altrimenti RunFakePIR lo comprimerebbe una seconda volta
	pi.Reset(DB, p)

	tputs := misura(pi, DB, p, 0, nRip)
	return calcola(fmt.Sprintf("Algorand senza cert (N=%d)", M), p, tputs)
}

// testLongRow: 2^16 record casuali da 8 KB, per confronto con i blocchi veri
func testLongRow(nRip int) risultato {
	logN := uint64(16)
	numKB := uint64(8)
	N := uint64(1 << logN)
	bit := uint64(numKB * 1024 * 8)

	pi := pir.SimplePIR{}
	p := pi.PickParams(N, bit, SEC_PARAM, LOGQ)
	DB := pir.MakeRandomDB(N, bit, &p)

	var tputs []float64
	for j := 0; j < nRip; j++ {
		tput, _ := pir.RunPIR(&pi, DB, p, []uint64{1})
		tputs = append(tputs, tput)
	}

	return calcola(fmt.Sprintf("LongRow Rand (2^%d, %dKB)", logN, numKB), p, tputs)
}

func main() {
	var risultati []risultato

	fmt.Println("Avvio benchmark...")

	// runtime.GC()
	// risultati = append(risultati, testLongRow(1))

	runtime.GC()
	risultati = append(risultati, testAlgorand("../blocchi_senza_cert", SOGLIA, 3))

	// la tabella va sia a schermo sia in fondo a risultati.txt
	f, err := os.OpenFile("risultati.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	out := io.MultiWriter(os.Stdout, f)

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%-30s %18s %11s %10s %10s %10s\n",
		"configurazione", "throughput (MB/s)", "answer (ms)", "hint (MB)", "query (KB)", "risp. (KB)")

	// valori del paper (AWS c5n.metal, DB da 1 GB); i 100 ms sono ricavati da 1 GB / 10 GB/s
	fmt.Fprintf(out, "%-30s %18s %11.3f %10.1f %10.1f %10.1f\n",
		"paper (c5n.metal)", "~10240", 100.0, 121.0, 121.0, 121.0)

	for _, r := range risultati {
		fmt.Fprintf(out, "%-30s %10.0f ± %5.0f %11.3f %10.1f %10.1f %10.1f\n",
			r.cfg, r.tput, r.tputStd, r.tempo, r.hintMB, r.upKB, r.downKB)
	}
}
