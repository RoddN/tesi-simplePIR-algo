import sys
from pathlib import Path

import pandas as pd

folder = Path(sys.argv[1] if len(sys.argv) > 1 else "BlocchiNoCert")

w = pd.Series([f.stat().st_size for f in folder.glob("*.msgpack")])

print(f"blocks: {len(w)}")
print(f"media: {w.mean():.0f} byte")
print(f"mediana: {w.median():.0f} byte")
print(f"min: {w.min()} byte")
print(f"max: {w.max()} byte")

print(f"Q1: {w.quantile(0.25):.0f} byte")
print(f"Q3: {w.quantile(0.75):.0f} byte")
print(f"P95: {w.quantile(0.95):.0f} byte")
print(f"P99: {w.quantile(0.99):.0f} byte")

iqr = w.quantile(0.75) - w.quantile(0.25)
print(f"limite IQR superiore: {w.quantile(0.75) + 1.5 * iqr:.0f} byte")
print(f"blocchi oltre P99: {(w > w.quantile(0.99)).sum()}")