# tesi-simplePIR-algo

Materiale di lavoro per la tesi: recupero privato di blocchi Algorand con SimplePIR.

- `benchmark/`: benchmark di SimplePIR sui blocchi (risultati in `risultati.txt` e `tempi.csv`)
- `server-client/`: server con un solo DB e client che recupera un blocco via HTTP
- `server-client-m/`: DB diviso in due sezioni, il client interroga entrambe (una con una query fittizia)
- `simplepir/`: copia della libreria [SimplePIR](https://github.com/ahenzinger/simplepir), usata tramite `replace` in `go.mod`
- `utils/`: decodifica msgpack -> JSON
- `cmd/msgpack_tool/`: stampa un blocco msgpack come JSON
- `utils_python/`: script per scaricare i blocchi (`estrai_blocchi.py`), togliere il certificato (`rimuovi_cert.py`) e calcolarne le statistiche (`summary.py`)

I blocchi (msgpack mainnet, senza certificato) non sono nel repository:
vanno messi in `BlocchiNoCert/` nella root, il codice li legge da `../BlocchiNoCert`.

## Avvio

Ogni programma va lanciato dalla sua cartella.

```bash
cd benchmark
go run . [etichetta]
```

```bash
cd server-client-m
go run ./server_m    # aspettare "Pronto ... su http://localhost:8080"
go run ./client_m    # in un altro terminale
```

`server-client/` funziona allo stesso modo (`go run ./server`, poi `go run ./client`).
