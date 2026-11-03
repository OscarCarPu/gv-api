package printers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

const historyLimit = 10

type PrintHistoryEntry struct {
	State          string   `json:"state"`
	TimePrinting   *float64 `json:"timePrinting,omitempty"`
	FileName       string   `json:"fileName"`
	Material       string   `json:"material,omitempty"`
	FilamentGrams  *float64 `json:"filamentGrams,omitempty"`
	FilamentMeters *float64 `json:"filamentMeters,omitempty"`
}

type rawConnectJob struct {
	State        string   `json:"state"`
	TimePrinting *float64 `json:"time_printing"`
	Progress     *float64 `json:"progress"`
	File         *struct {
		DisplayName string         `json:"display_name"`
		Name        string         `json:"name"`
		Meta        map[string]any `json:"meta"`
	} `json:"file"`
}

func parseConnectJobs(data []byte) ([]PrintHistoryEntry, error) {
	var wrapped struct {
		Jobs []rawConnectJob `json:"jobs"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	out := make([]PrintHistoryEntry, 0, len(wrapped.Jobs))
	for _, j := range wrapped.Jobs {
		e := PrintHistoryEntry{State: j.State, TimePrinting: j.TimePrinting}
		if j.File != nil {
			e.FileName = firstString(j.File.DisplayName, j.File.Name)
			m := parsePrintMeta(j.File.Meta)
			e.Material = m.Material
			done := doneFraction(j.State, j.Progress, j.TimePrinting, m.PrintSeconds)
			e.FilamentGrams = scaleUsed(m.FilamentGrams, done)
			e.FilamentMeters = scaleUsed(m.FilamentMeters, done)
		}
		out = append(out, e)
	}
	return out, nil
}

func (c *connectClient) history(ctx context.Context) ([]PrintHistoryEntry, error) {
	id, err := c.printerUUID(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"limit": {strconv.Itoa(historyLimit)}, "offset": {"0"}}
	data, err := c.get(ctx, fmt.Sprintf("/app/printers/%s/jobs?%s", url.PathEscape(id), q.Encode()))
	if err != nil {
		return nil, err
	}
	jobs, err := parseConnectJobs(data)
	if err != nil {
		return nil, &statusError{http.StatusBadGateway, "Prusa Connect returned something unexpected"}
	}
	if len(jobs) > historyLimit {
		jobs = jobs[:historyLimit]
	}
	return jobs, nil
}

func doneFraction(state string, progress, timePrinting *float64, estimate *int64) float64 {
	if state != "FIN_STOPPED" && state != "FIN_ERROR" {
		return 1
	}
	switch {
	case progress != nil && *progress > 0:
		return min(1, *progress/100)
	case timePrinting != nil && estimate != nil && *estimate > 0:
		return min(1, *timePrinting/float64(*estimate))
	}
	return 1
}

func scaleUsed(v *float64, done float64) *float64 {
	if v == nil || done >= 1 {
		return v
	}
	scaled := math.Round(*v*done*10) / 10
	return &scaled
}

const (
	connectFilesPage  = 100
	connectFilesPages = 10
	chamberCacheTTL   = 10 * time.Second
)

func (c *connectClient) fileMeta(ctx context.Context) (map[string]PrintMeta, error) {
	id, err := c.printerUUID(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]PrintMeta{}
	for page := range connectFilesPages {
		q := url.Values{"limit": {strconv.Itoa(connectFilesPage)}, "offset": {strconv.Itoa(page * connectFilesPage)}}
		data, err := c.get(ctx, fmt.Sprintf("/app/printers/%s/files?%s", url.PathEscape(id), q.Encode()))
		if err != nil {
			return nil, err
		}
		var res struct {
			Files []struct {
				Type string         `json:"type"`
				Path string         `json:"path"`
				Meta map[string]any `json:"meta"`
			} `json:"files"`
		}
		if json.Unmarshal(data, &res) != nil {
			return nil, &statusError{http.StatusBadGateway, "Prusa Connect returned something unexpected"}
		}
		for _, f := range res.Files {
			if f.Type == "PRINT_FILE" && f.Path != "" {
				out[path.Base(f.Path)] = parsePrintMeta(f.Meta)
			}
		}
		if len(res.Files) < connectFilesPage {
			break
		}
	}
	return out, nil
}

type Chamber struct {
	Temp       *float64 `json:"temp,omitempty"`
	TargetTemp *float64 `json:"targetTemp,omitempty"`
}

func (c *connectClient) chamber(ctx context.Context) (Chamber, error) {
	c.mu.Lock()
	cached, at := c.chamberCache, c.chamberAt
	c.mu.Unlock()
	if !at.IsZero() && time.Since(at) < chamberCacheTTL {
		return cached, nil
	}

	id, err := c.printerUUID(ctx)
	if err != nil {
		return Chamber{}, err
	}
	data, err := c.get(ctx, "/app/printers/"+url.PathEscape(id))
	if err != nil {
		return Chamber{}, err
	}
	var res struct {
		Chamber *struct {
			Temp       *float64 `json:"temp"`
			TargetTemp *float64 `json:"target_temp"`
		} `json:"chamber"`
	}
	if json.Unmarshal(data, &res) != nil {
		return Chamber{}, &statusError{http.StatusBadGateway, "Prusa Connect returned something unexpected"}
	}
	var out Chamber
	if res.Chamber != nil {
		out = Chamber{Temp: res.Chamber.Temp, TargetTemp: res.Chamber.TargetTemp}
	}
	c.mu.Lock()
	c.chamberCache, c.chamberAt = out, time.Now()
	c.mu.Unlock()
	return out, nil
}
