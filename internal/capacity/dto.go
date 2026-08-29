package capacity

import "github.com/shopspring/decimal"

type DayFreeBusy struct {
	Date          string          `json:"date"`           // YYYY-MM-DD, zona del servicio
	CapacityHours decimal.Decimal `json:"capacity_hours"` // igual todos los días
	BusyHours     decimal.Decimal `json:"busy_hours"`
	FreeHours     decimal.Decimal `json:"free_hours"` // max(capacity - busy, 0)
}

type FreeBusyRangeResponse struct {
	From string        `json:"from"`
	To   string        `json:"to"`
	Days []DayFreeBusy `json:"days"`
}
