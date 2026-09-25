# tesi-pir

Recupero di blocchi Algorand tramite PIR, usando la libreria
[SimplePIR](https://github.com/ahenzinger/simplepir) senza modificarla.
Il client chiede un blocco e il server glielo restituisce senza sapere quale ha chiesto.

Ci sono due programmi:

- `main.go`: versione ottimizzata, una query PIR per blocco
- `lento/main.go`: versione che usa la libreria nel modo standard, una query per ogni 8 byte del blocco

Servono a confrontare i tempi. Il codice è lo stesso tranne le due funzioni
descritte sotto.

## Dati

`blocchi/` contiene 1000 blocchi mainnet (round 64250205–64251204), presi da un
nodo archiver con algosdk e salvati uno per file come JSON. Vanno da 1,7 KB a
462 KB, la mediana è 27 KB.

## Uso

Serve Go e un compilatore C (la libreria usa cgo).

```
go build
go build -o lento/lento ./lento

./tesi-pir 44              # blocco 44, tenendo solo i blocchi fino a 32 KB
./tesi-pir 44 1000000      # blocco 44, tutti i 1000 blocchi
./lento/lento 44           # stesso test con la versione standard
```

Il secondo argomento è una soglia in byte: i blocchi più grandi vengono
scartati. Serve perché tutti i record del DB devono avere la stessa dimensione,
e con pochi blocchi enormi si spreca spazio. L'indice è la posizione tra i
blocchi rimasti dopo il filtro.

Alla fine viene stampato il round del blocco recuperato, la sua dimensione e un
OK/ERRORE che lo confronta con l'originale (client e server sono nello stesso
processo, quindi il confronto è possibile):

```
indice 44 -> round 64250302, 30020 byte, OK
```

## Come è fatto il DB

SimplePIR lavora su entry da al massimo 64 bit (`MakeDB` prende una lista di
`uint64` e `Recover` ne restituisce uno). Un blocco quindi viene tagliato in
pezzi da 8 caratteri, ognuno convertito in un numero. Tutti i blocchi vengono
portati alla lunghezza del più grande con spazi in coda, così ogni blocco
occupa lo stesso numero K di entry. Internamente la libreria spezza ogni entry
in 7 celle mod 991 (memorizzate come `uint32`), ma questo è trasparente.

## Perché la versione standard è lenta

Con 610 blocchi fino a 32 KB, K = 4089. Usando la libreria come viene, per
avere un blocco servono 4089 query separate, e ogni query fa moltiplicare al
server l'intera matrice (4179 × 4179) per un vettore: circa 4 ms. In totale 17
secondi per blocco.

Il fatto è che il server, per come funziona SimplePIR, risponde sempre con una
colonna intera della matrice. La `Recover` della libreria ne legge una riga sola
e butta via il resto. Quindi di quelle 4089 risposte si usa una cella su 4179
per ciascuna.

## Cosa cambia nella versione ottimizzata

1. **Un blocco per colonna.** Invece di lasciare che `PickParams` scelga una
   matrice quadrata, uso `PickParamsGivenDimensions` con M = numero di blocchi
   colonne e L = K × 7 righe, e metto le entry del blocco j in verticale nella
   colonna j (posizione `c*M + j` nella lista invece di `j*K + c`). Così il
   blocco j è esattamente la colonna j.

2. **Recover di tutta la colonna.** Ho scritto `recover_column`, che fa le
   stesse operazioni di `pi.Recover` (stesso offset, stessa sottrazione di
   `H·s`, stesso arrotondamento) ma su tutte le righe della risposta invece che
   su una. Restituisce le K entry del blocco.

Risultato: una query sola per blocco

## Tempi

Misurati su un PC Linux con 30 GB di RAM, sempre sul blocco di indice 44.
Setup = costruzione DB + hint, uguale nelle due versioni (dipende solo dal
numero di celle). La baseline è il DB con tutti i 1000 blocchi; le altre righe
scartano i blocchi sopra la soglia.

| soglia | blocchi | K | setup | query standard | query ottimizzata | rapporto |
|---|---|---|---|---|---|---|
| nessuna (baseline) | 1000 | 57776 | 23 s | ~50 min (stimato) | 1,40 s | ~2000× |
| 128 KB | 988 | 15012 | 6–9 s | 3 min 9 s | 0,24 s | 800× |
| 96 KB | 975 | 12209 | 5–7 s | 2 min 11 s | 0,15 s | 870× |
| 64 KB | 936 | 8172 | 3–4 s | 66 s | 61 ms | 1080× |
| 48 KB | 842 | 6106 | 2–3 s | 38 s | 36 ms | 1050× |
| 32 KB | 610 | 4089 | 1 s | 16,7 s | 14 ms | 1190× |
| 16 KB | 252 | 2042 | 0,2–0,3 s | 3,3 s | 4,7 ms | 700× |

La riga della baseline con la versione standard non è stata eseguita fino in
fondo: sono 57776 chiamate su una matrice da 404 milioni di celle, circa 50 ms
l'una. Il tempo è estrapolato dalle righe sopra.

Il rapporto è circa K: nella versione standard le query sono K, in quella
ottimizzata una. Il tempo della singola query cresce con il DB, che è
dominato dal padding: il blocco più grande è 462 KB contro una mediana di 27
KB, quindi senza soglia circa il 94% del DB è vuoto e la query lo paga (il
tempo è lineare nella dimensione del DB, è inevitabile con il PIR). Senza
soglia il programma usa 7,5 GB di RAM.

Una soglia ragionevole è 64 KB: tiene il 93% dei blocchi e la query sta sotto
i 100 ms. Oltre, ogni raddoppio di soglia recupera pochi blocchi e raddoppia il
padding.
