package blockpir

import "encoding/binary"

func EntriesPerBlock(blocks [][]byte) uint64 {
	max := 0
	for _, b := range blocks {
		if len(b) > max {
			max = len(b)
		}
	}
	return uint64((max + 4 + 7) / 8)
}

func EncodeBlock(block []byte, K uint64) []uint64 {
	buf := make([]byte, K*8)

	binary.BigEndian.PutUint32(buf, uint32(len(block)))
	copy(buf[4:], block)

	out := make([]uint64, K)
	for i := range out {
		out[i] = binary.BigEndian.Uint64(buf[i*8:])
	}
	return out
}

func DecodeBlock(entries []uint64) []byte {
	buf := make([]byte, len(entries)*8)
	for i, val := range entries {
		binary.BigEndian.PutUint64(buf[i*8:], val)
	}
	n := binary.BigEndian.Uint32(buf)

	if n > uint32(len(entries)*8-4) {
		return nil
	}
	return buf[4 : 4+n]
}
