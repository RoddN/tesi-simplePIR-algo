package blockpir

import "github.com/ahenzinger/simplepir/pir"

func ReseedPRG(pi pir.SimplePIR) {
	pi.InitCompressed(pir.DBinfo{}, pir.Params{M: 1, N: 1, Logq: LogQ})
}
