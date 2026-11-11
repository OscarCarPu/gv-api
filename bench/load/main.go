// Command load measures gv-api endpoint latency and throughput. It only needs the API's base URL
// and JWT secret, so it runs unchanged against any implementation of the same contract.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type endpoint struct {
	Name, Method, Path, Body string
	Kind                     string // token kind; "" means none
	Expect                   int    // expected status when not 2xx
	DeletePath               string // write cycle: POST, then DELETE DeletePath with {id}
}

type result struct {
	Scenario    string  `json:"scenario"`
	Name        string  `json:"name"`
	Method      string  `json:"method"`
	Path        string  `json:"path"`
	Status      int     `json:"status"`
	Bytes       int     `json:"bytes"`
	N           int     `json:"n"`
	P50         float64 `json:"p50_ms"`
	P95         float64 `json:"p95_ms"`
	P99         float64 `json:"p99_ms"`
	Max         float64 `json:"max_ms"`
	Mean        float64 `json:"mean_ms"`
	DeleteP50   float64 `json:"delete_p50_ms,omitempty"`
	DeleteP95   float64 `json:"delete_p95_ms,omitempty"`
	Concurrency int     `json:"concurrency,omitempty"`
	RPS         float64 `json:"rps,omitempty"`
	ConcP95     float64 `json:"conc_p95_ms,omitempty"`
	Errors      int     `json:"errors"`
}

var (
	base     = flag.String("url", "http://localhost:18080", "API base URL")
	secret   = flag.String("secret", os.Getenv("JWT_SECRET"), "JWT secret")
	scenario = flag.String("scenario", "", "scenario label for the output")
	n        = flag.Int("n", 100, "sequential requests per endpoint")
	warm     = flag.Int("warm", 10, "warm-up requests per endpoint")
	conc     = flag.Int("c", 1, "concurrency for an extra throughput pass (1 = none)")
	concN    = flag.Int("cn", 400, "requests per endpoint in the throughput pass")
	openTask = flag.Int("task", 1, "id of an open task")
	project  = flag.Int("project", 1, "id of a root project")
	only     = flag.String("only", "", "comma-separated endpoint names to run")
)

var client = &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 64}}

func token(kind string) string {
	enc := base64.RawURLEncoding
	head := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	now := time.Now().Unix()
	claims, _ := json.Marshal(map[string]any{"kind": kind, "iat": now, "exp": now + 3600})
	msg := head + "." + enc.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte(*secret))
	mac.Write([]byte(msg))
	return msg + "." + enc.EncodeToString(mac.Sum(nil))
}

func do(method, path, body, kind string) (int, []byte, time.Duration, error) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, *base+path, rd)
	if err != nil {
		return 0, nil, 0, err
	}
	if kind != "" {
		req.Header.Set("Authorization", "Bearer "+token(kind))
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, time.Since(start), err
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, b, time.Since(start), err
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func pct(s []float64, p float64) float64 {
	if len(s) == 0 {
		return 0
	}
	i := int(p*float64(len(s)) + 0.5)
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i]
}

func summarize(lat []float64) (p50, p95, p99, mx, mean float64) {
	sort.Float64s(lat)
	var sum float64
	for _, v := range lat {
		sum += v
	}
	return pct(lat, .50), pct(lat, .95), pct(lat, .99), lat[len(lat)-1], sum / float64(len(lat))
}

func idOf(b []byte) string {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	if v, ok := m["id"].(float64); ok {
		return strconv.Itoa(int(v))
	}
	return ""
}

func ok(code int, e endpoint) bool { return code == e.Expect || (code >= 200 && code < 300) }

func runRead(e endpoint) result {
	r := result{Scenario: *scenario, Name: e.Name, Method: e.Method, Path: e.Path}
	for i := 0; i < *warm; i++ {
		_, _, _, _ = do(e.Method, e.Path, e.Body, e.Kind)
	}
	lat := make([]float64, 0, *n)
	for i := 0; i < *n; i++ {
		code, b, d, err := do(e.Method, e.Path, e.Body, e.Kind)
		r.Status, r.Bytes = code, len(b)
		if err != nil || !ok(code, e) {
			r.Errors++
		}
		lat = append(lat, ms(d))
	}
	r.N = len(lat)
	r.P50, r.P95, r.P99, r.Max, r.Mean = summarize(lat)

	if *conc > 1 && e.Method == http.MethodGet {
		var mu sync.Mutex
		var clat []float64
		jobs := make(chan struct{}, *concN)
		for i := 0; i < *concN; i++ {
			jobs <- struct{}{}
		}
		close(jobs)
		var wg sync.WaitGroup
		start := time.Now()
		for w := 0; w < *conc; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range jobs {
					code, _, d, err := do(e.Method, e.Path, e.Body, e.Kind)
					mu.Lock()
					clat = append(clat, ms(d))
					if err != nil || !ok(code, e) {
						r.Errors++
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		r.Concurrency = *conc
		r.RPS = float64(len(clat)) / time.Since(start).Seconds()
		_, r.ConcP95, _, _, _ = summarize(clat)
	}
	return r
}

func runWrite(e endpoint) result {
	r := result{Scenario: *scenario, Name: e.Name, Method: e.Method, Path: e.Path}
	var lat, dlat []float64
	for i := 0; i < *n+*warm; i++ {
		code, b, d, err := do(e.Method, e.Path, e.Body, e.Kind)
		r.Status, r.Bytes = code, len(b)
		if err != nil || !ok(code, e) {
			r.Errors++
			continue
		}
		id := idOf(b)
		if i >= *warm {
			lat = append(lat, ms(d))
		}
		if e.DeletePath == "" || id == "" {
			continue
		}
		code, _, d, err = do(http.MethodDelete, strings.ReplaceAll(e.DeletePath, "{id}", id), "", e.Kind)
		if err != nil || !ok(code, e) {
			r.Errors++
		} else if i >= *warm {
			dlat = append(dlat, ms(d))
		}
	}
	if len(lat) == 0 {
		return r
	}
	r.N = len(lat)
	r.P50, r.P95, r.P99, r.Max, r.Mean = summarize(lat)
	if len(dlat) > 0 {
		r.DeleteP50, r.DeleteP95, _, _, _ = summarize(dlat)
	}
	return r
}

func endpoints() []endpoint {
	now := time.Now()
	day := func(off int) string { return now.AddDate(0, 0, off).Format("2006-01-02") }
	ts := func(off time.Duration) string { return now.Add(off).UTC().Format(time.RFC3339) }
	month := func(off int) string { return now.AddDate(0, off, 0).Format("2006-01") }
	task, proj := strconv.Itoa(*openTask), strconv.Itoa(*project)
	F, S := "full", "semi"

	reads := []endpoint{
		{Name: "health", Method: "GET", Path: "/health"},
		{Name: "habits.daily", Path: "/habits?date=" + day(0), Kind: F},
		{Name: "habits.history.1y", Path: "/habits/1/history?frequency=daily&start_at=" + day(-365) + "&end_at=" + day(0), Kind: F},
		{Name: "tasks.tree", Path: "/tasks/tree", Kind: F},
		{Name: "tasks.projects.roots", Path: "/tasks/projects", Kind: F},
		{Name: "tasks.projects.list-fast", Path: "/tasks/projects/list-fast", Kind: F},
		{Name: "tasks.tasks.list-fast", Path: "/tasks/tasks/list-fast", Kind: F},
		{Name: "tasks.project", Path: "/tasks/projects/" + proj, Kind: F},
		{Name: "tasks.project.children", Path: "/tasks/projects/" + proj + "/children", Kind: F},
		{Name: "tasks.project.parent-candidates", Path: "/tasks/projects/" + proj + "/parent-candidates", Kind: F},
		{Name: "tasks.task", Path: "/tasks/tasks/" + task, Kind: F},
		{Name: "tasks.task.time-entries", Path: "/tasks/tasks/" + task + "/time-entries", Kind: F},
		{Name: "tasks.by-due-date", Path: "/tasks/tasks/by-due-date", Kind: F},
		{Name: "tasks.by-due-date.slack", Path: "/tasks/tasks/by-due-date/slack", Kind: F},
		{Name: "tasks.time-entries.week", Path: "/tasks/time-entries?start_time=" + day(-7), Kind: F},
		{Name: "tasks.time-entries.active", Path: "/tasks/time-entries/active", Kind: F, Expect: 404},
		{Name: "tasks.time-entries.summary", Path: "/tasks/time-entries/summary", Kind: F},
		{Name: "tasks.time-entries.history.30d", Path: "/tasks/time-entries/history?frequency=daily&start_at=" + day(-30) + "&end_at=" + day(0), Kind: F},
		{Name: "tasks.time-entries.history.5y", Path: "/tasks/time-entries/history?frequency=monthly&start_at=" + day(-1900) + "&end_at=" + day(0), Kind: F},
		{Name: "plan.today", Path: "/plan/today", Kind: F},
		{Name: "plan.range.2w", Path: "/plan/range?from=" + day(-7) + "&to=" + day(7), Kind: F},
		{Name: "plan.commitments", Path: "/plan/commitments", Kind: F},
		{Name: "capacity.30d", Path: "/capacity/free-busy?from=" + day(0) + "&to=" + day(30), Kind: F},
		{Name: "calendar.month", Path: "/calendar/events?from=" + ts(-15*24*time.Hour) + "&to=" + ts(15*24*time.Hour), Kind: F},
		{Name: "finance.accounts", Path: "/finance/accounts", Kind: F},
		{Name: "finance.categories", Path: "/finance/categories", Kind: F},
		{Name: "finance.overview", Path: "/finance/overview", Kind: F},
		{Name: "finance.transactions.all", Path: "/finance/transactions", Kind: F},
		{Name: "finance.transactions.month", Path: "/finance/transactions?from=" + day(-30) + "&to=" + day(0), Kind: F},
		{Name: "finance.budgets", Path: "/finance/budgets?month=" + month(0), Kind: F},
		{Name: "finance.stats.monthly.1y", Path: "/finance/stats/monthly?from=" + day(-365) + "&to=" + day(0), Kind: F},
		{Name: "finance.stats.monthly.5y", Path: "/finance/stats/monthly?from=" + day(-1900) + "&to=" + day(0), Kind: F},
		{Name: "finance.stats.by-category.1y", Path: "/finance/stats/by-category?type=expense&from=" + day(-365) + "&to=" + day(0), Kind: F},
		{Name: "finance.stats.networth.1y", Path: "/finance/stats/networth?from=" + day(-365) + "&to=" + day(0) + "&granularity=week", Kind: F},
		{Name: "finance.stats.networth.5y", Path: "/finance/stats/networth?from=" + day(-1900) + "&to=" + day(0) + "&granularity=month", Kind: F},
		{Name: "finance.stats.estimation", Path: "/finance/stats/estimation?start_month=" + month(-12) + "&end_month=" + month(0) + "&mode=rate", Kind: F},
		{Name: "rutas.marks", Path: "/rutas/marks", Kind: S},
		{Name: "uptime", Path: "/domotics/uptime", Kind: S},
		{Name: "uptime.windows.30d", Path: "/domotics/uptime/windows?from=" + ts(-30*24*time.Hour) + "&to=" + ts(0), Kind: S},
	}
	for i := range reads {
		if reads[i].Method == "" {
			reads[i].Method = "GET"
		}
	}
	start := now.Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	end := now.Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	// Plan blocks reject overlaps; this slot is past every seeded block.
	bStart := now.AddDate(0, 0, 700).UTC().Format(time.RFC3339)
	bEnd := now.AddDate(0, 0, 700).Add(time.Hour).UTC().Format(time.RFC3339)
	writes := []endpoint{
		{Name: "w.habits.log", Method: "POST", Path: "/habits/log", Kind: F, Body: `{"habit_id":1,"date":"` + day(0) + `","value":2}`},
		{Name: "w.finance.transaction", Method: "POST", Path: "/finance/transactions", Kind: F, DeletePath: "/finance/transactions/{id}",
			Body: `{"type":"expense","amount":"12.34","account_id":1,"category_id":4,"description":"bench"}`},
		{Name: "w.tasks.task", Method: "POST", Path: "/tasks/tasks", Kind: F, DeletePath: "/tasks/tasks/{id}",
			Body: `{"name":"bench","project_id":` + proj + `}`},
		{Name: "w.tasks.time-entry", Method: "POST", Path: "/tasks/time-entries", Kind: F, DeletePath: "/tasks/time-entries/{id}",
			Body: `{"task_id":` + task + `,"started_at":"` + start + `","finished_at":"` + end + `"}`},
		{Name: "w.plan.block", Method: "POST", Path: "/plan/blocks", Kind: F, DeletePath: "/plan/blocks/{id}",
			Body: `{"started_at":"` + bStart + `","ended_at":"` + bEnd + `","label":"bench"}`},
		{Name: "w.calendar.event", Method: "POST", Path: "/calendar/events", Kind: F, DeletePath: "/calendar/events/{id}",
			Body: `{"name":"bench","start_time":"` + start + `","end_time":"` + end + `"}`},
	}
	return append(reads, writes...)
}

func main() {
	flag.Parse()
	want := map[string]bool{}
	for _, s := range strings.Split(*only, ",") {
		if s != "" {
			want[s] = true
		}
	}
	enc := json.NewEncoder(os.Stdout)
	for _, e := range endpoints() {
		if len(want) > 0 && !want[e.Name] {
			continue
		}
		var r result
		if e.Method == "GET" {
			r = runRead(e)
		} else {
			r = runWrite(e)
		}
		_ = enc.Encode(r)
		fmt.Fprintf(os.Stderr, "%-34s %3d p50 %7.2f p95 %7.2f ms  %7.0f rps  %6d B  err %d\n",
			r.Name, r.Status, r.P50, r.P95, r.RPS, r.Bytes, r.Errors)
	}
}
