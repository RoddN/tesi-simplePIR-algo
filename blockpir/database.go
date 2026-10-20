package blockpir

import "github.com/ahenzinger/simplepir/pir"

const (
	bitsPerEntry = 64
	SecParam     = 1024
	LogQ         = 32
)

func BuildDB(pi pir.SimplePIR, blocks [][]byte) (*pir.Database, pir.Params, uint64) {
	K := EntriesPerBlock(blocks)
	DB, p := BuildDBWithK(pi, blocks, K, uint64(len(blocks)))
	return DB, p, K
}

// noiseM è il numero di colonne su cui si accumula il rumore: con le risposte
// degli shard sommate è il totale dei blocchi, non quelli del singolo shard.
func BuildDBWithK(pi pir.SimplePIR, blocks [][]byte, K, noiseM uint64) (*pir.Database, pir.Params) {
	M := uint64(len(blocks))

	p := pi.PickParamsGivenDimensions(1, noiseM, SecParam, LogQ)
	Ne := pir.Compute_num_entries_base_p(p.P, bitsPerEntry)
	p = pi.PickParamsGivenDimensions(K*Ne, noiseM, SecParam, LogQ)
	p.M = M

	vals := make([]uint64, K*M)
	for j, b := range blocks {
		for c, val := range EncodeBlock(b, K) {
			vals[uint64(c)*M+uint64(j)] = val
		}
	}
	return pir.MakeDB(K*M, bitsPerEntry, &p, vals), p
}

func DecodeAnswer(p pir.Params, info pir.DBinfo, H pir.Msg, query pir.Msg, answer pir.Msg, secret pir.State, K uint64) []uint64 {
	return DecodeSum(p, info, []pir.Msg{H}, []pir.Msg{query}, answer, []pir.State{secret}, K)
}

// Decodifica la somma delle risposte di più database alle rispettive query:
// si tolgono tutti gli H_j·s_j e l'offset di p/2 accumulato su tutte le query.
func DecodeSum(p pir.Params, info pir.DBinfo, H []pir.Msg, queries []pir.Msg, answer pir.Msg, secrets []pir.State, K uint64) []uint64 {
	ans := answer.Data[0]

	offset := uint64(0)
	for _, q := range queries {
		for j := uint64(0); j < p.M; j++ {
			offset += (p.P / 2) * q.Data[0].Get(j, 0)
		}
	}
	offset = (1 << p.Logq) - (offset % (1 << p.Logq))

	Hs := pir.MatrixMul(H[0].Data[0], secrets[0].Data[0])
	for j := 1; j < len(H); j++ {
		Hs.MatrixAdd(pir.MatrixMul(H[j].Data[0], secrets[j].Data[0]))
	}
	ans.MatrixSub(Hs)

	entries := make([]uint64, K)
	for c := uint64(0); c < K; c++ {
		var digits []uint64
		for r := c * info.Ne; r < (c+1)*info.Ne; r++ {
			digits = append(digits, p.Round(ans.Get(r, 0)+offset))
		}
		entries[c] = pir.ReconstructElem(digits, c, info)
	}
	ans.MatrixAdd(Hs)
	return entries
}
