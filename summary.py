import sys
from pathlib import Path

import pandas as pd

folder = Path(sys.argv[1] if len(sys.argv) > 1 else "blocchi_senza_cert")

w = pd.Series([f.stat().st_size for f in folder.glob("*.msgpack")])

print(f"blocks: {len(w)}")
print(f"media: {w.mean():.0f} byte")
print(f"mediana: {w.median():.0f} byte")
print(f"min: {w.min()} byte")
print(f"max: {w.max()} byte")


