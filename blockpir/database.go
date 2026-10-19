package blockpir

import "github.com/ahenzinger/simplepir/pir"

const (
	bitsPerEntry = 64
	SecParam     = 1024
	LogQ         = 32
)

func BuildDB(pi pir.SimplePIR, blocks [][]byte) (*pir.Database, pir.Params, uint64) {
	K := EntriesPerBlock(blocks)
	DB, p := BuildDBWithK(pi, blocks, K)
	return DB, p, K
}

func BuildDBWithK(pi pir.SimplePIR, blocks [][]byte, K uint64) (*pir.Database, pir.Params) {
	M := uint64(len(blocks))

	p := pi.PickParamsGivenDimensions(1, M, SecParam, LogQ)
	Ne := pir.Compute_num_entries_base_p(p.P, bitsPerEntry)
	p = pi.PickParamsGivenDimensions(K*Ne, M, SecParam, LogQ)

	vals := make([]uint64, K*M)
	for j, b := range blocks {
		for c, val := range EncodeBlock(b, K) {
			vals[uint64(c)*M+uint64(j)] = val
		}
	}
	return pir.MakeDB(K*M, bitsPerEntry, &p, vals), p
}

func DecodeAnswer(p pir.Params, info pir.DBinfo, H pir.Msg, query pir.Msg, answer pir.Msg, secret pir.State, K uint64) []uint64 {
	s := secret.Data[0]
	ans := answer.Data[0]

	offset := uint64(0)
	for j := uint64(0); j < p.M; j++ {
		offset += (p.P / 2) * query.Data[0].Get(j, 0)
	}
	offset = (1 << p.Logq) - (offset % (1 << p.Logq))

	Hs := pir.MatrixMul(H.Data[0], s)
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
