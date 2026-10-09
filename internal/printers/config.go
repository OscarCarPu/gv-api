package printers

import "strings"

type Printer struct {
	ID       string
	Name     string
	Model    string
	RTSP     string
	Host     string
	User     string
	Password string
	APIKey   string
	Storage  string
}

type Config struct {
	Printers      []Printer
	RecordingsDir string
	UploadsDir    string
	MaxMinutes    int
	MaxBytes      int64
	VideoCodec    string
	Overlay       bool
	Font          string
	SignKey       []byte
}

func (c Config) printer(id string) (Printer, bool) {
	for _, p := range c.Printers {
		if p.ID == id {
			return p, true
		}
	}
	return Printer{}, false
}

func trimSlashes(s string) string {
	return strings.Trim(s, "/")
}
