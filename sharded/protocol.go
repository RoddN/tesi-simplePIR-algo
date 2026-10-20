package sharded

import (
	"time"

	"tesi-simplepir/metrics"

	"github.com/ahenzinger/simplepir/pir"
)

type Hint struct {
	Params    pir.Params
	Info      pir.DBinfo
	Seed      pir.CompressedState
	H         []pir.Msg
	K         uint64
	ShardSize int
	NumBlocks int
	Stats     metrics.SetupStats
}

type Response struct {
	Answers    []pir.Msg
	ServerTime time.Duration
}

func ShardSize() int {
	return (NumBlocks + NumShards - 1) / NumShards
}
