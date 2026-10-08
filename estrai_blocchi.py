import os
from algosdk.v2client import algod

# data_dir = "/home/giuper/node/data1"
out_dir = "./Blocchi"

# with open (f"{data_dir}/algod.token", "r") as f :
#	token = f.read().strip()

# with open (f"{data_dir}/algod.net", "r") as f:
#    address = f.read().strip()

# url = f"http://{address}"

url = "https://mainnet-api.algonode.cloud"
token= ""

client = algod.AlgodClient(token, url)
status = client.status()
last = status["last-round"]
numberOfBlocks = 80000
start = last - numberOfBlocks

os.makedirs(out_dir, exist_ok=True)
print(f"Estraggo dal round {start} al {last}...")

for n,i in enumerate(range(start, last)):
	raw = client.block_info(i, response_format='msgpack')
	with open (f"{out_dir}/blocco_{n:04d}_round_{i}.msgpack", "wb") as f:
		f.write(raw)
	if i% 100 == 0:
		print(f"Scaricato blocco {i}")
print("Finito")
