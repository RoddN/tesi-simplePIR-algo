package singledb

import (
	"time"

	"tesi-simplepir/metrics"

	"github.com/ahenzinger/simplepir/pir"
)

type Hint struct {
	Params pir.Params
	Info   pir.DBinfo

	Seed  pir.CompressedState
	H     pir.Msg
	K     uint64
	M     int
	Stats metrics.SetupStats
}

type Response struct {
	Answer     pir.Msg
	ServerTime time.Duration
}
