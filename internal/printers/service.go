package printers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

var bg = context.Background()

type unit struct {
	p    Printer
	link *link
	cam  *camera
	rec  *recorder
}

type PublicPrinter struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

type Service struct {
	cfg     Config
	units   map[string]*unit
	uploads *uploads
}

func NewService(cfg Config) *Service {
	s := &Service{cfg: cfg, units: map[string]*unit{}, uploads: newUploads(cfg.UploadsDir)}
	for _, p := range cfg.Printers {
		l := &link{p: p}
		s.units[p.ID] = &unit{p: p, link: l, cam: &camera{url: p.RTSP}, rec: &recorder{cfg: cfg, p: p, link: l}}
	}
	return s
}

func (s *Service) List() []PublicPrinter {
	out := []PublicPrinter{}
	for _, p := range s.cfg.Printers {
		out = append(out, PublicPrinter{p.ID, p.Name, p.Model})
	}
	return out
}

func (s *Service) unit(id string) (*unit, bool) {
	u, ok := s.units[id]
	return u, ok
}

func (s *Service) recordings(un *unit) RecordingsView {
	list := un.rec.list(un.rec.liveName())
	view := RecordingsView{Recordings: list, MaxBytes: s.cfg.MaxBytes}
	for i := range list {
		view.UsedBytes += list[i].SizeBytes
		list[i].URL = s.mediaURL(un.p.ID, list[i].Name)
		if list[i].Poster != "" {
			list[i].PosterURL = s.mediaURL(un.p.ID, list[i].Poster)
		}
	}
	return view
}

func newUploadID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
