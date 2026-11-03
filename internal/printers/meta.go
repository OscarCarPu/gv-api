package printers

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type PrintMeta struct {
	PrintSeconds   *int64   `json:"printSeconds,omitempty"`
	Material       string   `json:"material,omitempty"`
	FilamentGrams  *float64 `json:"filamentGrams,omitempty"`
	FilamentMeters *float64 `json:"filamentMeters,omitempty"`
}

func (f *File) hasMeta() bool {
	return f.PrintSeconds != nil || f.Material != "" || f.FilamentGrams != nil || f.FilamentMeters != nil
}

var (
	timeKeys  = []string{"estimated_print_time", "estimated printing time (normal mode)", "estimated_printing_time_normal_mode", "print_time", "printing_time"}
	gramKeys  = []string{"filament used [g]", "filament_used_g", "filament_used [g]"}
	mmKeys    = []string{"filament used [mm]", "filament_used_mm", "filament_used [mm]"}
	durationP = regexp.MustCompile(`(\d+)\s*([dhms])`)
)

func parsePrintMeta(meta map[string]any) PrintMeta {
	var out PrintMeta
	if len(meta) == 0 {
		return out
	}
	for _, k := range timeKeys {
		if v, ok := meta[k]; ok {
			if secs, ok := durationSeconds(v); ok {
				out.PrintSeconds = &secs
				break
			}
		}
	}
	if t, ok := meta["filament_type"].(string); ok {
		out.Material = materialList(t)
	}
	for _, k := range gramKeys {
		if g, ok := sumNumbers(meta[k]); ok {
			g = math.Round(g*10) / 10
			out.FilamentGrams = &g
			break
		}
	}
	for _, k := range mmKeys {
		if mm, ok := sumNumbers(meta[k]); ok {
			m := math.Round(mm/10) / 100
			out.FilamentMeters = &m
			break
		}
	}
	if out.FilamentMeters == nil {
		if m, ok := sumNumbers(meta["filament_used_m"]); ok {
			m = math.Round(m*100) / 100
			out.FilamentMeters = &m
		}
	}
	return out
}

func durationSeconds(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), t > 0
	case string:
		s := strings.TrimSpace(t)
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(n), n > 0
		}
		var total int64
		for _, m := range durationP.FindAllStringSubmatch(s, -1) {
			n, _ := strconv.ParseInt(m[1], 10, 64)
			switch m[2] {
			case "d":
				total += n * 86400
			case "h":
				total += n * 3600
			case "m":
				total += n * 60
			case "s":
				total += n
			}
		}
		return total, total > 0
	}
	return 0, false
}

func sumNumbers(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, t > 0
	case string:
		var total float64
		found := false
		for _, part := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ';' }) {
			if n, err := strconv.ParseFloat(strings.TrimSpace(part), 64); err == nil {
				total += n
				found = true
			}
		}
		return total, found && total > 0
	}
	return 0, false
}

func materialList(raw string) string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		p := strings.TrimSpace(part)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}
