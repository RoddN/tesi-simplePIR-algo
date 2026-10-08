package utils
import (
	"encoding/binary"
	"os"

	"github.com/ahenzinger/simplepir/pir"
)

const (
	d         = 64
	SEC_PARAM = 1024
	LOGQ      = 32
	SOGLIA    = 24958 // Taglio outlier
	DIR       = "/home/giuper/Nocera/tesi-simplePIR-algo/BlocchiNoCert"
	WARMUP    = 2
	RIP       = 30
	M = 10000
)

func ReadBloks(dir string, maxLen int) [][]byte {
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
	return buf[4 : 4+n]
}

func BuildDB(pi pir.SimplePIR, blocks [][]byte) (*pir.Database, pir.Params, uint64) {
	M := uint64(len(blocks))
	K := FindK(blocks)

	p := pi.PickParamsGivenDimensions(1, M, SEC_PARAM, LOGQ)
	Ne := pir.Compute_num_entries_base_p(p.P, d)
	p = pi.PickParamsGivenDimensions(K*Ne, M, SEC_PARAM, LOGQ)

	vals := make([]uint64, K*M)
	for j, b := range blocks {
		for c, val := range Pack(b, K) {
			vals[uint64(c)*M+uint64(j)] = val
		}
	}
	return pir.MakeDB(K*M, d, &p, vals), p, K
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