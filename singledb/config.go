package singledb

const (
	BlocksDir     = "../BlocchiNoCert"
	MaxBlockBytes = 24958
	NumBlocks     = 200000
	RepeatBlocks  = true

	WarmupRuns   = 2
	MeasuredRuns = 30

	HintFile   = "hint.gob"
	ResultsDir = "results"

	ListenAddr = ":8080"
	ServerURL  = "http://localhost:8080/query"
)
