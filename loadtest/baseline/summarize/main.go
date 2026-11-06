// Command summarize turns naive-baseline run directories (loadtest/baseline/run.sh)
// into Markdown tables: one row per recorded run, then per concurrency level the
// median and range across runs. Warm-up runs are listed separately and are not
// evidence.
//
//	go run ./loadtest/baseline/summarize loadtest/results
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type key struct {
	RunID       string  `json:"run_id"`
	Buyers      int     `json:"buyers"`
	Duration    string  `json:"duration"`
	Requests    float64 `json:"requests"`
	RPS         float64 `json:"requests_per_second"`
	Booked      float64 `json:"bookings_succeeded"`
	Rejected    float64 `json:"bookings_rejected_unavailable"`
	Failed      float64 `json:"bookings_failed_other"`
	ErrorRate   float64 `json:"error_rate"`
	Dropped     float64 `json:"dropped_iterations"`
	Latency     lat     `json:"latency_ms"`
	LatBooked   lat     `json:"latency_booked_ms"`
	LatRejected lat     `json:"latency_rejected_ms"`
}

type lat struct {
	P50 *float64 `json:"p50"`
	P95 *float64 `json:"p95"`
	P99 *float64 `json:"p99"`
	Max *float64 `json:"max"`
}

type run struct {
	dir    string
	warmup bool
	k      key
	audit  map[string]float64
	cpu    map[string]float64 // CPU seconds used during the run, per service
	rss    map[string]float64 // peak RSS in MiB, per service
	pgCPU  float64            // peak Postgres container CPU %
	srvMS  map[string]float64 // server-side mean latency of the baseline route, per service
	// Booking pool: share of acquires that waited, and mean wait per acquire.
	poolShare, poolMS float64
	samples           int // resource samples captured (per service)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: summarize <results-dir>")
		os.Exit(2)
	}
	dirs, _ := filepath.Glob(filepath.Join(os.Args[1], "*-b*-*"))
	sort.Strings(dirs)
	var runs []run
	for _, d := range dirs {
		r, err := load(d)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", d, err)
			continue
		}
		runs = append(runs, r)
	}
	printRuns("Recorded runs", runs, false)
	printLevels(runs)
	printRuns("Warm-up runs (not evidence)", runs, true)
}

func load(dir string) (run, error) {
	r := run{dir: filepath.Base(dir), warmup: strings.HasSuffix(dir, "-warmup"), cpu: map[string]float64{}, rss: map[string]float64{}, srvMS: map[string]float64{}}
	b, err := os.ReadFile(filepath.Join(dir, "k6-key.json"))
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r.k); err != nil {
		return r, err
	}
	if r.audit, err = readAudit(filepath.Join(dir, "audit-after.csv")); err != nil {
		return r, err
	}
	readProcess(filepath.Join(dir, "process-samples.csv"), &r)
	r.pgCPU = readPGPeak(filepath.Join(dir, "postgres-stats.csv"))
	for _, svc := range []string{"gateway", "booking"} {
		r.srvMS[svc] = serverMeanMS(filepath.Join(dir, "metrics-"+svc+"-before.txt"), filepath.Join(dir, "metrics-"+svc+"-after.txt"))
	}
	r.poolShare, r.poolMS = poolWait(filepath.Join(dir, "metrics-booking-before.txt"), filepath.Join(dir, "metrics-booking-after.txt"))
	return r, nil
}

// serverMeanMS is the exact mean latency of the baseline route during the run,
// from the RED histogram's _sum and _count deltas (after minus before).
func serverMeanMS(before, after string) float64 {
	read := func(path string) (sum, count float64) {
		b, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(b), "\n") {
			if !strings.Contains(line, `route="POST /v1/baseline/events/{eventID}/holds"`) {
				continue
			}
			fields := strings.Fields(line)
			v, _ := strconv.ParseFloat(fields[len(fields)-1], 64)
			switch {
			case strings.HasPrefix(line, "http_request_duration_seconds_sum{"):
				sum = v
			case strings.HasPrefix(line, "http_request_duration_seconds_count{"):
				count = v
			}
		}
		return
	}
	s0, c0 := read(before)
	s1, c1 := read(after)
	if c1-c0 <= 0 {
		return 0
	}
	return (s1 - s0) / (c1 - c0) * 1000
}

func readCSV(path string) [][]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	rows, _ := cr.ReadAll()
	return rows
}

func readAudit(path string) (map[string]float64, error) {
	rows := readCSV(path)
	if len(rows) < 2 {
		return nil, fmt.Errorf("no audit in %s", path)
	}
	m := map[string]float64{}
	for i, h := range rows[0] {
		m[h], _ = strconv.ParseFloat(rows[1][i], 64)
	}
	return m, nil
}

func readProcess(path string, r *run) {
	first := map[string]float64{}
	for _, row := range readCSV(path) {
		if len(row) < 4 || row[0] == "unix_ts" || row[2] == "" {
			continue
		}
		svc := row[1]
		cpu, err1 := strconv.ParseFloat(row[2], 64)
		mem, err2 := strconv.ParseFloat(row[3], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if _, ok := first[svc]; !ok {
			first[svc] = cpu
		}
		r.cpu[svc] = cpu - first[svc]
		r.samples++
		if mib := mem / (1 << 20); mib > r.rss[svc] {
			r.rss[svc] = mib
		}
	}
	r.samples /= 2 // one row per service per sample
}

// poolWait reads Booking's pgxpool counters (before vs after): the share of
// connection acquires that had to wait, and the mean wait per acquire.
func poolWait(before, after string) (share, meanMS float64) {
	read := func(path string) map[string]float64 {
		m := map[string]float64{}
		b, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(b), "\n") {
			if fields := strings.Fields(line); len(fields) == 2 && strings.HasPrefix(fields[0], "pgxpool_") {
				m[fields[0]], _ = strconv.ParseFloat(fields[1], 64)
			}
		}
		return m
	}
	b, a := read(before), read(after)
	acq := a["pgxpool_acquire_total"] - b["pgxpool_acquire_total"]
	if acq <= 0 {
		return 0, 0
	}
	return (a["pgxpool_empty_acquire_total"] - b["pgxpool_empty_acquire_total"]) / acq,
		(a["pgxpool_acquire_duration_seconds_total"] - b["pgxpool_acquire_duration_seconds_total"]) / acq * 1000
}

// res prints a resource figure, or "–" when the sampler did not capture at
// least two samples (a delta needs two).
func (r run) res(v float64, prec int) string {
	if r.samples < 2 {
		return "–"
	}
	return fmt.Sprintf("%.*f", prec, v)
}

func readPGPeak(path string) float64 {
	peak := 0.0
	for _, row := range readCSV(path) {
		if len(row) < 3 || row[0] == "unix_ts" {
			continue
		}
		if v, err := strconv.ParseFloat(strings.TrimSuffix(row[2], "%"), 64); err == nil && v > peak {
			peak = v
		}
	}
	return peak
}

func f(p *float64) string {
	if p == nil {
		return "–"
	}
	return fmt.Sprintf("%.1f", *p)
}

func printRuns(title string, runs []run, warmup bool) {
	fmt.Printf("### %s\n\n", title)
	fmt.Println("| Run | Buyers | Requests | Req/s | 201 booked | 409 rejected | Other errors | Error rate | p50 ms | p95 ms | p99 ms | Max ms | Double-booked seats | Excess seat bookings | Max bookings/seat | Seats left | Gateway mean ms (server) | Booking mean ms (server) | Pool acquires that waited | Mean pool wait ms | Booking CPU s | Booking peak RSS MiB | Gateway CPU s | Postgres peak CPU % | Samples |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, r := range runs {
		if r.warmup != warmup {
			continue
		}
		k, a := r.k, r.audit
		fmt.Printf("| `%s` | %d | %.0f | %.0f | %.0f | %.0f | %.0f | %.4f | %s | %s | %s | %s | %.0f | %.0f | %.0f | %.0f | %.1f | %.1f | %.1f%% | %.1f | %s | %s | %s | %s | %d |\n",
			r.dir, k.Buyers, k.Requests, k.RPS, k.Booked, k.Rejected, k.Failed, k.ErrorRate,
			f(k.Latency.P50), f(k.Latency.P95), f(k.Latency.P99), f(k.Latency.Max),
			a["double_booked_seats"], a["excess_seat_bookings"], a["max_bookings_per_seat"], a["seats_available"],
			r.srvMS["gateway"], r.srvMS["booking"], 100*r.poolShare, r.poolMS,
			r.res(r.cpu["booking"], 1), r.res(r.rss["booking"], 0), r.res(r.cpu["gateway"], 1), r.res(r.pgCPU, 0), r.samples)
	}
	fmt.Println()
}

func printLevels(runs []run) {
	by := map[int][]run{}
	for _, r := range runs {
		if !r.warmup {
			by[r.k.Buyers] = append(by[r.k.Buyers], r)
		}
	}
	levels := make([]int, 0, len(by))
	for b := range by {
		levels = append(levels, b)
	}
	sort.Ints(levels)
	fmt.Println("### Per concurrency level: median [min – max] across recorded runs")
	fmt.Println()
	fmt.Println("| Buyers | Runs | Req/s | 201 booked | Error rate | p50 ms | p95 ms | p99 ms | Double-booked seats | Excess seat bookings |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|")
	for _, b := range levels {
		rs := by[b]
		col := func(get func(run) float64, prec int) string {
			v := make([]float64, 0, len(rs))
			for _, r := range rs {
				v = append(v, get(r))
			}
			sort.Float64s(v)
			med := v[len(v)/2]
			if len(v)%2 == 0 {
				med = (v[len(v)/2-1] + v[len(v)/2]) / 2
			}
			return fmt.Sprintf("%.*f [%.*f – %.*f]", prec, med, prec, v[0], prec, v[len(v)-1])
		}
		deref := func(p *float64) float64 {
			if p == nil {
				return 0
			}
			return *p
		}
		fmt.Printf("| %d | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n", b, len(rs),
			col(func(r run) float64 { return r.k.RPS }, 0),
			col(func(r run) float64 { return r.k.Booked }, 0),
			col(func(r run) float64 { return r.k.ErrorRate }, 4),
			col(func(r run) float64 { return deref(r.k.Latency.P50) }, 1),
			col(func(r run) float64 { return deref(r.k.Latency.P95) }, 1),
			col(func(r run) float64 { return deref(r.k.Latency.P99) }, 1),
			col(func(r run) float64 { return r.audit["double_booked_seats"] }, 0),
			col(func(r run) float64 { return r.audit["excess_seat_bookings"] }, 0))
	}
	fmt.Println()
}
