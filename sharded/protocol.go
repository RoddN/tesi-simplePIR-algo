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

// Una query per shard: quella vera per lo shard del blocco, query a zero per gli altri
type Request struct {
	Queries []pir.Msg
}

// Somma delle risposte di tutti gli shard
type Response struct {
	Answer     pir.Msg
	ServerTime time.Duration
}

func ShardSize() int {
	return (NumBlocks + NumShards - 1) / NumShards
}
