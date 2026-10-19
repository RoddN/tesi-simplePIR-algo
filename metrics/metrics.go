package metrics

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"text/tabwriter"
	"time"
)

type SetupStats struct {
	UniqueBlocks int
	RawBytes     int64
	DBBytes      int64
	DBTime       time.Duration
	HintTime     time.Duration
}

type Run struct {
	Block         uint64
	Query         time.Duration
	Server        time.Duration
	RTT           time.Duration
	Decode        time.Duration
	UploadBytes   int
	DownloadBytes int
}

type Summary struct {
	Label      string
	M          int
	Shards     int
	HBytes     int64
	ABytes     int64
	WarmupRuns int
	Setup      SetupStats
}

func Save(dir string, s Summary, runs []Run) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := appendSummary(dir, s, runs); err != nil {
		return err
	}
	return appendRuns(dir, s, runs)
}

func appendSummary(dir string, s Summary, runs []Run) error {
	f, err := os.OpenFile(filepath.Join(dir, "summary.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := tabwriter.NewWriter(f, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "\n--- %s, %s, %d run (+%d di riscaldamento) ---\n",
		s.Label, time.Now().Format("2006-01-02 15:04"), len(runs), s.WarmupRuns)
	fmt.Fprintln(w, "M\tShard\tUnici\tDati\tDB\tH\tA\tTempo DB\tTempo hint\tQuery\tServer\tRTT\tDecifratura\tUpload\tDownload")

	fmt.Fprintf(w, "%d\t%d\t%d\t%s\t%s\t%s\t%s\t%.2f s\t%.2f s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		s.M, s.Shards, s.Setup.UniqueBlocks,
		size(s.Setup.RawBytes), size(s.Setup.DBBytes), size(s.HBytes), size(s.ABytes),
		s.Setup.DBTime.Seconds(), s.Setup.HintTime.Seconds(),
		timing(runs, func(r Run) time.Duration { return r.Query }),
		timing(runs, func(r Run) time.Duration { return r.Server }),
		timing(runs, func(r Run) time.Duration { return r.RTT }),
		timing(runs, func(r Run) time.Duration { return r.Decode }),
		size(int64(runs[0].UploadBytes)), size(int64(runs[0].DownloadBytes)))
	return w.Flush()
}

func appendRuns(dir string, s Summary, runs []Run) error {
	path := filepath.Join(dir, "runs.csv")

	_, statErr := os.Stat(path)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if os.IsNotExist(statErr) {
		w.Write([]string{"label", "M", "run", "block", "query_ms", "server_ms", "rtt_ms", "decode_ms", "upload_B", "download_B"})
	}
	for i, r := range runs {
		w.Write([]string{
			s.Label, strconv.Itoa(s.M), strconv.Itoa(i), strconv.FormatUint(r.Block, 10),
			ms(r.Query), ms(r.Server), ms(r.RTT), ms(r.Decode),
			strconv.Itoa(r.UploadBytes), strconv.Itoa(r.DownloadBytes),
		})
	}
	w.Flush()
	return w.Error()
}

func ms(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds()*1000, 'f', 4, 64)
}

func size(b int64) string {
	if b >= 1<<20 {
		return fmt.Sprintf("%.2f MB", float64(b)/(1<<20))
	}
	return fmt.Sprintf("%.2f KB", float64(b)/(1<<10))
}

func timing(runs []Run, get func(Run) time.Duration) string {
	v := make([]float64, len(runs))
	for i, r := range runs {
		v[i] = get(r).Seconds() * 1000
	}
	return fmt.Sprintf("%.2f ± %.2f ms", median(v), stddev(v))
}

func median(v []float64) float64 {
	c := slices.Clone(v)
	slices.Sort(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func stddev(v []float64) float64 {
	mean := 0.0
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))

	sum := 0.0
	for _, x := range v {
		sum += (x - mean) * (x - mean)
	}
	return math.Sqrt(sum / float64(len(v)))
}
