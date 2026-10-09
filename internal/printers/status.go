package printers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type Telemetry struct {
	Configured bool     `json:"configured"`
	Online     bool     `json:"online"`
	State      string   `json:"state,omitempty"`
	Temps      Temps    `json:"temps"`
	Fans       Fans     `json:"fans"`
	AxisZ      *float64 `json:"axisZ,omitempty"`
	Speed      *float64 `json:"speed,omitempty"`
	Flow       *float64 `json:"flow,omitempty"`
	Job        *JobInfo `json:"job,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type Temps struct {
	Nozzle       *float64 `json:"nozzle,omitempty"`
	NozzleTarget *float64 `json:"nozzleTarget,omitempty"`
	Bed          *float64 `json:"bed,omitempty"`
	BedTarget    *float64 `json:"bedTarget,omitempty"`
	Chamber      *float64 `json:"chamber,omitempty"`
}

type Fans struct {
	Hotend *float64 `json:"hotend,omitempty"`
	Print  *float64 `json:"print,omitempty"`
}

type JobInfo struct {
	Progress      *float64 `json:"progress,omitempty"`
	TimeRemaining *float64 `json:"timeRemaining,omitempty"`
	TimePrinting  *float64 `json:"timePrinting,omitempty"`
	FileName      string   `json:"fileName,omitempty"`
	Material      string   `json:"material,omitempty"`
}

type rawJob struct {
	ID            int      `json:"id"`
	Progress      *float64 `json:"progress"`
	TimeRemaining *float64 `json:"time_remaining"`
	TimePrinting  *float64 `json:"time_printing"`
	File          struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Meta        struct {
			FilamentType string `json:"filament_type"`
		} `json:"meta"`
	} `json:"file"`
}

type rawStatus struct {
	Printer struct {
		State        string   `json:"state"`
		TempNozzle   *float64 `json:"temp_nozzle"`
		TargetNozzle *float64 `json:"target_nozzle"`
		TempBed      *float64 `json:"temp_bed"`
		TargetBed    *float64 `json:"target_bed"`
		TempChamber  *float64 `json:"temp_chamber"`
		AxisZ        *float64 `json:"axis_z"`
		Speed        *float64 `json:"speed"`
		Flow         *float64 `json:"flow"`
		FanHotend    *float64 `json:"fan_hotend"`
		FanPrint     *float64 `json:"fan_print"`
	} `json:"printer"`
	Job *rawJob `json:"job"`
}

type rawLegacy struct {
	Telemetry struct {
		Material string `json:"material"`
	} `json:"telemetry"`
}

func (l *link) getJSON(ctx context.Context, path string, out any) (bool, error) {
	status, data, err := l.do(ctx, request{
		method:  http.MethodGet,
		path:    path,
		headers: map[string]string{"Accept": "application/json"},
		timeout: shortTimeout,
	})
	if err != nil {
		return false, err
	}
	if status == http.StatusNoContent {
		return false, nil
	}
	if status < 200 || status > 299 {
		return false, fmt.Errorf("PrusaLink %s -> %d", path, status)
	}
	return true, json.Unmarshal(data, out)
}

func (l *link) status(ctx context.Context) Telemetry {
	t := Telemetry{Configured: l.p.Host != ""}
	if !t.Configured {
		return t
	}

	var (
		st      rawStatus
		job     rawJob
		legacy  rawLegacy
		hasJob  bool
		wg      sync.WaitGroup
		statErr error
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, statErr = l.getJSON(ctx, "/api/v1/status", &st)
	}()
	go func() {
		defer wg.Done()
		hasJob, _ = l.getJSON(ctx, "/api/v1/job", &job)
	}()
	go func() {
		defer wg.Done()
		_, _ = l.getJSON(ctx, "/api/printer", &legacy)
	}()
	wg.Wait()

	if statErr != nil {
		t.Error = statErr.Error()
		return t
	}

	p := st.Printer
	t.Online = true
	t.State = p.State
	t.Temps = Temps{p.TempNozzle, p.TargetNozzle, p.TempBed, p.TargetBed, p.TempChamber}
	t.Fans = Fans{p.FanHotend, p.FanPrint}
	t.AxisZ, t.Speed, t.Flow = p.AxisZ, p.Speed, p.Flow

	var j *rawJob
	if hasJob {
		j = &job
	} else {
		j = st.Job
	}
	if j == nil {
		return t
	}
	t.Job = &JobInfo{
		Progress:      firstFloat(j.Progress, jobOf(st).Progress),
		TimeRemaining: firstFloat(j.TimeRemaining, jobOf(st).TimeRemaining),
		TimePrinting:  firstFloat(j.TimePrinting, jobOf(st).TimePrinting),
		FileName:      firstString(j.File.DisplayName, j.File.Name),
		Material:      firstString(j.File.Meta.FilamentType, legacy.Telemetry.Material),
	}
	return t
}

func jobOf(st rawStatus) rawJob {
	if st.Job == nil {
		return rawJob{}
	}
	return *st.Job
}

func firstFloat(a, b *float64) *float64 {
	if a != nil {
		return a
	}
	return b
}

func firstString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
