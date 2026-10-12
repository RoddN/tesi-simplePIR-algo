package main

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	utils "tesi-simplepir/server-client-m/utils_m"

	"github.com/ahenzinger/simplepir/pir"
)

// Una sezione del DB con il suo stato PIR
type ServerSection struct {
	pi          pir.SimplePIR
	DB          *pir.Database
	p           pir.Params
	A           pir.State
	serverState pir.State
}

var sezioni []ServerSection

func main() {
	debug.SetGCPercent(-1)

	fmt.Println("[SERVER] Carico i blocchi")
	blocks, _ := utils.ReadBlocks(utils.DIR, utils.SOGLIA)

	// 40.000 blocchi = 2 sezioni da CHUNK_SIZE
	targetM := 40000
	if len(blocks) > targetM {
		blocks = blocks[:targetM]
	} else if len(blocks) < targetM {
		fmt.Printf("[SERVER] Attenzione: solo %d blocchi su %d\n", len(blocks), targetM)
	}

	fmt.Printf("[SERVER] %d blocchi, setup delle sezioni in parallelo\n\n", len(blocks))

	numSections := (len(blocks) + utils.CHUNK_SIZE - 1) / utils.CHUNK_SIZE
	sezioni = make([]ServerSection, numSections)

	var wg sync.WaitGroup
	startAll := time.Now()

	for i := 0; i < numSections; i++ {
		wg.Add(1)

		// Una goroutine per sezione
		go func(idx int) {
			defer wg.Done()

			start := idx * utils.CHUNK_SIZE
			end := start + utils.CHUNK_SIZE
			if end > len(blocks) {
				end = len(blocks)
			}
			chunkBlocks := blocks[start:end]

			fmt.Printf("[Sezione %d] Setup DB (%d blocchi)\n", idx, len(chunkBlocks))

			pi := pir.SimplePIR{}

			startDB := time.Now()
			DB, p, K := utils.MakeDB(pi, chunkBlocks)
			dbTime := time.Since(startDB)

			startHint := time.Now()
			A := pi.Init(DB.Info, p)
			serverState, H := pi.Setup(DB, A, p)
			hintTime := time.Since(startHint)

			fmt.Printf("[Sezione %d] DB in %v, hint in %v\n", idx, dbTime, hintTime)

			// Hint per il client
			hintFilename := fmt.Sprintf("client_hint_%d.gob", idx)
			file, _ := os.Create(hintFilename)

			setupData := utils.ClientSetupData{
				Params: p,
				Info:   DB.Info,
				A:      A,
				H:      H,
				K:      K,
				M:      len(chunkBlocks),
			}
			gob.NewEncoder(file).Encode(setupData)
			file.Close()

			sezioni[idx] = ServerSection{
				pi:          pi,
				DB:          DB,
				p:           p,
				A:           A,
				serverState: serverState,
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("\n[SERVER] Pronto in %v su http://localhost:8080\n", time.Since(startAll))

	http.HandleFunc("/query", handleQuery)
	http.ListenAndServe(":8080", nil)
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	var req utils.MultiQueryRequest
	err := gob.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "query non valida", http.StatusBadRequest)
		return
	}

	// Risponde a entrambe: non sa quale query è quella reale
	ans0 := sezioni[0].pi.Answer(sezioni[0].DB, pir.MakeMsgSlice(req.QueryDB1), sezioni[0].serverState, sezioni[0].A, sezioni[0].p)
	ans1 := sezioni[1].pi.Answer(sezioni[1].DB, pir.MakeMsgSlice(req.QueryDB2), sezioni[1].serverState, sezioni[1].A, sezioni[1].p)

	resp := utils.MultiQueryResponse{
		AnswerDB1: ans0,
		AnswerDB2: ans1,
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	gob.NewEncoder(w).Encode(resp)
}
