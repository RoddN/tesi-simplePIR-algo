package main

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/ahenzinger/simplepir/pir"
	"tesi-simplepir/utils"
)

type ClientSetupData struct {
	Params pir.Params
	Info   pir.DBinfo
	A      pir.State
	H      pir.Msg
	K      uint64
	M      int
}

var (
	pi          pir.SimplePIR
	DB          *pir.Database
	serverState pir.State
	p           pir.Params
	A           pir.State
)

func handleQuery(w http.ResponseWriter, r *http.Request) {
	var q pir.Msg
	gob.NewDecoder(r.Body).Decode(&q)

	batch := pir.MakeMsgSlice(q)
	ans := pi.Answer(DB, batch, serverState, A, p)

	w.Header().Set("Content-Type", "application/octet-stream")
	gob.NewEncoder(w).Encode(ans)
	fmt.Println("[SERVER] Response sent to client.")
}

func main() {
	debug.SetGCPercent(-1)

	fmt.Println("[SERVER] Loading DB...")
	blocks := utils.ReadBloks(utils.DIR, utils.SOGLIA)[:utils.M]

	pi = pir.SimplePIR{}
	var K uint64
	DB, p, K = utils.BuildDB(pi, blocks)

	fmt.Println("[SERVER] Building Hint...")
	A = pi.Init(DB.Info, p)
	var H pir.Msg
	serverState, H = pi.Setup(DB, A, p)

	fmt.Println("[SERVER] Saving Hint in 'client_hint.gob'...")
	file, err := os.Create("client_hint.gob")
	if err != nil {
		panic("Error creating Hint: " + err.Error())
	}

	setupData := ClientSetupData{
		Params: p,
		Info:   DB.Info,
		A:      A,
		H:      H,
		K:      K,
		M:      utils.M,
	}
	gob.NewEncoder(file).Encode(setupData)
	file.Close()

	fmt.Println("[SERVER] Ready on http://localhost:8080")
	http.HandleFunc("/query", handleQuery)
	http.ListenAndServe(":8080", nil)
}
