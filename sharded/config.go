package sharded

const (
	BlocksDir     = "../BlocchiNoCert"
	MaxBlockBytes = 24958
	NumBlocks     = 200000
	RepeatBlocks  = true
	NumShards     = 2

	WarmupRuns   = 2
	MeasuredRuns = 30

	ParallelAnswers = true

	HintFile   = "hint.gob"
	ResultsDir = "results"

	ListenAddr = ":8080"
	ServerURL  = "http://localhost:8080/query"
)
