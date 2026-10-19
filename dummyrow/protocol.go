package dummyrow

import (
	"tesi-simplepir/blockpir"

	"github.com/ahenzinger/simplepir/pir"
)

type Hint struct {
	Params pir.Params
	Info   pir.DBinfo
	A      pir.State
	H      pir.Msg
	K      uint64
	M      int
}

type ShardedQuery struct {
	Queries []pir.Msg
}

type ShardedResponse struct {
	Answers []pir.Msg
}

func NumShards() int {
	return (NumBlocks + ShardSize - 1) / ShardSize
}

func BuildShardDB(pi pir.SimplePIR, blocks [][]byte) (*pir.Database, pir.Params, uint64) {
	withDummy := append([][]byte{{}}, blocks...)
	return blockpir.BuildDB(pi, withDummy)
}
