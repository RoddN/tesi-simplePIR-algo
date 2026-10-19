import os
from algosdk.v2client import algod

out_dir = "./Blocchi"

url = "https://mainnet-api.algonode.cloud"
token= ""

client = algod.AlgodClient(token, url)
status = client.status()
last = status["last-round"]
numberOfBlocks = 80000
start = last - numberOfBlocks

os.makedirs(out_dir, exist_ok=True)
print(f"Estraggo dal round {start} al {last}...")

for n in range(start, last):
    raw = client.block_info(n, response_format='msgpack')
    with open (f"{out_dir}/{n:04d}.msgpack", "wb") as f:
        f.write(raw)
    if n % 100 == 0:
        print(f"Scaricato blocco {n}")
print("Finito")
