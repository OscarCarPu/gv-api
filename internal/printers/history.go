package printers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const historyLimit = 10

type PrintHistoryEntry struct {
	State        string   `json:"state"`
	TimePrinting *float64 `json:"timePrinting,omitempty"`
	FileName     string   `json:"fileName"`
	Material     string   `json:"material,omitempty"`
}

type rawConnectJob struct {
	State        string   `json:"state"`
	TimePrinting *float64 `json:"time_printing"`
	File         *struct {
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
		Meta        *struct {
			FilamentType string `json:"filament_type"`
		} `json:"meta"`
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
			if j.File.Meta != nil {
				e.Material = j.File.Meta.FilamentType
			}
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
