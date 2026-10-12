"""Rimuove il certificato dai blocchi Algorand in formato msgpack.

Ogni file contiene una mappa {"block": ..., "cert": ...}. Lo script scrive in
una nuova cartella la mappa {"block": ...}, copiando i byte del blocco cosi'
come sono (senza decodificarli e ricodificarli), e poi verifica il risultato.

uso: python3 rimuovi_cert.py [cartella_ingresso] [cartella_uscita]
"""

import os
import sys

import msgpack


def estrai_block(raw):
    """Restituisce i byte msgpack del valore associato a "block"."""
    u = msgpack.Unpacker(raw=True, strict_map_key=False)
    u.feed(raw)
    n = u.read_map_header()
    for _ in range(n):
        chiave = u.unpack()
        inizio = u.tell()
        u.skip()
        if chiave == b"block":
            return raw[inizio:u.tell()]
    raise ValueError("campo block mancante")


def main():
    ingresso = sys.argv[1] if len(sys.argv) > 1 else "blocchi"
    uscita = sys.argv[2] if len(sys.argv) > 2 else "blocchi_senza_cert"
    os.makedirs(uscita, exist_ok=True)

    prima = dopo = 0
    nomi = sorted(os.listdir(ingresso))
    for nome in nomi:
        with open(os.path.join(ingresso, nome), "rb") as f:
            raw = f.read()

        # {"block": <byte originali del blocco>}
        nuovo = b"\x81" + msgpack.packb("block") + estrai_block(raw)

        # verifica: il blocco scritto deve essere uguale a quello originale
        orig = msgpack.unpackb(raw, raw=True, strict_map_key=False)
        pulito = msgpack.unpackb(nuovo, raw=True, strict_map_key=False)
        altre = set(orig) - {b"block", b"cert"}
        if altre:
            print(f"{nome}: chiavi inattese {altre}")
        if list(pulito) != [b"block"] or pulito[b"block"] != orig[b"block"]:
            raise RuntimeError(f"{nome}: blocco diverso dopo la pulizia")

        with open(os.path.join(uscita, nome), "wb") as f:
            f.write(nuovo)
        prima += len(raw)
        dopo += len(nuovo)

    print(f"{len(nomi)} blocchi: {prima / 1e6:.1f} MB -> {dopo / 1e6:.1f} MB "
          f"({100 * dopo / prima:.0f}%), salvati in {uscita}/")


if __name__ == "__main__":
    main()
