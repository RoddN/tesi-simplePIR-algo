# tesi-simplePIR-algo

Codice tesi per il recupero privato di blocchi Algorand con SimplePIR.

## Struttura

- `blockpir/`: codice PIR in comune
- `singledb/`: server e client con un unico database
- `sharded/`: server e client con il database diviso in shard
- `metrics/`: misure e scrittura dei risultati
- `benchmark/`: benchmark di SimplePIR senza rete
- `tools/`: script Python per preparare i blocchi e `msgpack2json`
- `dummyrow/`: versione precedente di sharded, non più usata

## Dati

I blocchi vanno in `BlocchiNoCert/`, nella root del progetto:

```bash
python3 tools/python/download_blocks.py                  # scarica i blocchi in Blocchi/
python3 tools/python/strip_cert.py Blocchi BlocchiNoCert # toglie i certificati
python3 tools/python/block_stats.py BlocchiNoCert        # statistiche sulle dimensioni
```

## Esecuzione

Server e client in due terminali separati:

```bash
cd singledb              
go run ./server          
go run ./client M10k     # M10k = etichetta dei risultati
```

I parametri (numero di blocchi, shard, run, porta) sono in `config.go`.

Benchmark senza rete:

```bash
cd benchmark
go run . run1
```

I risultati finiscono in `results/`.
