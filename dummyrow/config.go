package dummyrow

const (
	BlocksDir     = "../BlocchiNoCert"
	MaxBlockBytes = 24958
	NumBlocks     = 40000
	ShardSize     = 20000

	HintFilePattern = "hint_%d.gob"

	ListenAddr = ":8080"
	ServerURL  = "http://localhost:8080/query"
)
