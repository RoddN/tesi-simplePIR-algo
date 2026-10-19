package blockpir

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
)

func LoadBlocks(dir string, maxBytes, n int) ([][]byte, []string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("cartella %s non trovata: controllare il percorso dei blocchi nei parametri e la cartella da cui si lancia", dir)
	}

	var blocks [][]byte
	var names []string
	for _, f := range files {
		if n > 0 && len(blocks) == n {
			break
		}
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, nil, err
		}
		if len(b) <= maxBytes {
			blocks = append(blocks, b)
			names = append(names, f.Name())
		}
	}

	if len(blocks) < n || len(blocks) == 0 {
		return nil, nil, fmt.Errorf("solo %d blocchi entro %d byte, ne servono %d", len(blocks), maxBytes, n)
	}
	return blocks, names, nil
}

func RepeatBlocks(blocks [][]byte, names []string, n int) ([][]byte, []string) {
	if n <= len(blocks) {
		return blocks[:n], names[:n]
	}

	outBlocks := make([][]byte, n)
	outNames := make([]string, n)
	for i := 0; i < n; i++ {
		outBlocks[i] = blocks[i%len(blocks)]
		outNames[i] = names[i%len(names)]
	}
	return outBlocks, outNames
}

func LoadRepeated(dir string, maxBytes, n int, repeat bool) ([][]byte, int, error) {
	if !repeat {
		blocks, _, err := LoadBlocks(dir, maxBytes, n)
		return blocks, len(blocks), err
	}

	all, names, err := LoadBlocks(dir, maxBytes, 0)
	if err != nil {
		return nil, 0, err
	}
	blocks, _ := RepeatBlocks(all, names, n)
	return blocks, min(len(all), n), nil
}

func SaveGob(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return gob.NewEncoder(f).Encode(v)
}

func LoadGob(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return gob.NewDecoder(f).Decode(v)
}
