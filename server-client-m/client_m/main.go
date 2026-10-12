package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	utils "tesi-simplepir/server-client-m/utils_m"
	msgpack "tesi-simplepir/utils"

	"github.com/ahenzinger/simplepir/pir"
)

func main() {
	fmt.Println("[CLIENT] Avvio")

	// Servono solo per verificare il blocco ricevuto
	allBlocks, rounds := utils.ReadBlocks(utils.DIR, utils.SOGLIA)
	if len(allBlocks) > 40000 {
		allBlocks = allBlocks[:40000]
		rounds = rounds[:40000]
	}

	fmt.Println("[CLIENT] Carico gli hint")
	var setup0, setup1 utils.ClientSetupData

	f0, _ := os.Open("client_hint_0.gob")
	gob.NewDecoder(f0).Decode(&setup0)
	f0.Close()

	f1, _ := os.Open("client_hint_1.gob")
	gob.NewDecoder(f1).Decode(&setup1)
	f1.Close()

	pi := pir.SimplePIR{}

	// Le query fittizie puntano sempre all'indice 0, quindi si preparano prima
	fmt.Println("[CLIENT] Preparo le query fittizie")
	startPrecomp := time.Now()

	_, dummyQuery0 := pi.Query(0, setup0.A, setup0.Params, setup0.Info)
	_, dummyQuery1 := pi.Query(0, setup1.A, setup1.Params, setup1.Info)

	fmt.Printf("[CLIENT] Query fittizie pronte in %v\n\n", time.Since(startPrecomp))

	globalIdx := rand.Uint64N(uint64(len(allBlocks)))

	dbIndex := int(globalIdx) / utils.CHUNK_SIZE
	localIdx := (int(globalIdx) % utils.CHUNK_SIZE) + 1 // l'indice 0 è il blocco fittizio

	fmt.Printf("[CLIENT] Blocco %d (round %s) -> DB %d, indice %d\n", globalIdx, rounds[globalIdx], dbIndex, localIdx)

	var req utils.MultiQueryRequest
	var cliReal pir.State
	var realQuery pir.Msg
	var targetK uint64
	var targetSetup utils.ClientSetupData

	// Query reale al DB che contiene il blocco, fittizia all'altro
	startQueryGen := time.Now()

	if dbIndex == 0 {
		cliReal, realQuery = pi.Query(uint64(localIdx), setup0.A, setup0.Params, setup0.Info)
		req.QueryDB1 = realQuery
		req.QueryDB2 = dummyQuery1
		targetK = setup0.K
		targetSetup = setup0
	} else {
		cliReal, realQuery = pi.Query(uint64(localIdx), setup1.A, setup1.Params, setup1.Info)
		req.QueryDB1 = dummyQuery0
		req.QueryDB2 = realQuery
		targetK = setup1.K
		targetSetup = setup1
	}

	queryGenTime := time.Since(startQueryGen)

	var queryBuf bytes.Buffer
	gob.NewEncoder(&queryBuf).Encode(req)

	fmt.Println("--- Metriche ---")
	fmt.Printf("Generazione query: %v\n", queryGenTime)
	fmt.Printf("Upload (2 query):  %.2f KB\n", float64(queryBuf.Len())/1024.0)

	startNetwork := time.Now()
	respHTTP, err := http.Post("http://localhost:8080/query", "application/octet-stream", &queryBuf)
	if err != nil {
		panic("server non raggiungibile: " + err.Error())
	}

	var respMulti utils.MultiQueryResponse
	gob.NewDecoder(respHTTP.Body).Decode(&respMulti)
	respHTTP.Body.Close()

	networkTime := time.Since(startNetwork)
	fmt.Printf("Query time (RTT):  %v\n\n", networkTime)

	// Decodifico solo la risposta del DB interrogato
	var ansblock []byte

	if dbIndex == 0 {
		ansblock = utils.Unpack(utils.RecoverColumn(targetSetup.Params, targetSetup.Info, targetSetup.H, realQuery, respMulti.AnswerDB1, cliReal, targetK))
	} else {
		ansblock = utils.Unpack(utils.RecoverColumn(targetSetup.Params, targetSetup.Info, targetSetup.H, realQuery, respMulti.AnswerDB2, cliReal, targetK))
	}

	if !bytes.Equal(ansblock, allBlocks[globalIdx]) {
		fmt.Println("[CLIENT] Errore: blocco diverso dall'originale")
		return
	}

	jsonStr, _ := msgpack.DecodeMsgpackToJson(ansblock)
	fmt.Println("[CLIENT] Blocco recuperato")

	if len(jsonStr) > 250 {
		fmt.Printf("Estratto: %s...\n", jsonStr[50:250])
	} else {
		fmt.Println("Estratto:", jsonStr)
	}
}
