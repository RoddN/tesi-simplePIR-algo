package utils

import (
	"encoding/binary"
	"os"

	"github.com/ahenzinger/simplepir/pir"
)

const (
	D          = 64
	SEC_PARAM  = 1024
	LOGQ       = 32
	SOGLIA     = 24958
	DIR        = "../BlocchiNoCert"
	CHUNK_SIZE = 20000 // blocchi per sezione
)

// Setup di una sezione, scritto dal server e letto dal client
type ClientSetupData struct {
	Params pir.Params
	Info   pir.DBinfo
	A      pir.State
	H      pir.Msg
	K      uint64
	M      int // blocchi reali, senza quello fittizio
}

// Una query per sezione
type MultiQueryRequest struct {
	QueryDB1 pir.Msg
	QueryDB2 pir.Msg
}

// Una risposta per sezione, nello stesso ordine
type MultiQueryResponse struct {
	AnswerDB1 pir.Msg
	AnswerDB2 pir.Msg
}

func ReadBlocks(dir string, maxLen int) ([][]byte, []string) {
	var blocks [][]byte
	var rounds []string
	files, _ := os.ReadDir(dir)

	for _, f := range files {
		b, _ := os.ReadFile(dir + "/" + f.Name())
		if len(b) <= maxLen {
			blocks = append(blocks, b)
			rounds = append(rounds, f.Name())
		}
	}
	return blocks, rounds
}

func FindK(blocks [][]byte) uint64 {
	max := 0
	for _, b := range blocks {
		if len(b) > max {
			max = len(b)
		}
	}
	return uint64((max + 4 + 7) / 8) // +4 byte per la lunghezza
}

func Pack(block []byte, K uint64) []uint64 {
	buf := make([]byte, K*8)
	binary.BigEndian.PutUint32(buf, uint32(len(block)))
	copy(buf[4:], block)

	out := make([]uint64, K)
	for i := range out {
		out[i] = binary.BigEndian.Uint64(buf[i*8:])
	}
	return out
}

func Unpack(entries []uint64) []byte {
	buf := make([]byte, len(entries)*8)
	for i, val := range entries {
		binary.BigEndian.PutUint64(buf[i*8:], val)
	}
	n := binary.BigEndian.Uint32(buf)
	// Lunghezza non valida: non è un blocco reale
	if n == 0 || n > uint32(len(entries)*8-4) {
		return []byte{}
	}
	return buf[4 : 4+n]
}

// Mette un blocco di zeri in posizione 0, a cui puntano le query fittizie
func InjectDummyBlock(blocks [][]byte, K uint64) [][]byte {
	dummySize := K * 8
	dummyBlock := make([]byte, dummySize)

	res := make([][]byte, 0, len(blocks)+1)
	res = append(res, dummyBlock)
	res = append(res, blocks...)
	return res
}

func MakeDB(pi pir.SimplePIR, blocks [][]byte) (*pir.Database, pir.Params, uint64) {
	// K sui soli blocchi reali
	K := FindK(blocks)

	finalBlocks := InjectDummyBlock(blocks, K)
	M := uint64(len(finalBlocks)) // incluso il blocco fittizio

	p := pi.PickParamsGivenDimensions(1, M, SEC_PARAM, LOGQ)
	Ne := pir.Compute_num_entries_base_p(p.P, D)
	p = pi.PickParamsGivenDimensions(K*Ne, M, SEC_PARAM, LOGQ)

	vals := make([]uint64, K*M)
	for j, b := range finalBlocks {
		for c, val := range Pack(b, K) {
			vals[uint64(c)*M+uint64(j)] = val
		}
	}
	return pir.MakeDB(K*M, D, &p, vals), p, K
}

func RecoverColumn(p pir.Params, info pir.DBinfo, H pir.Msg, query pir.Msg, answer pir.Msg, client pir.State, K uint64) []uint64 {
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
