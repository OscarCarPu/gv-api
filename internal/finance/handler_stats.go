package finance

import (
	"net/http"

	"gv-api/internal/core"
	"gv-api/internal/finance/txtype"
)

// GetOverview -> GET /finance/overview
func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
	o, err := h.service.GetOverview(r.Context())
	if err != nil {
		core.InternalError(w, r, err, "Failed to get overview")
		return
	}
	core.JSON(w, http.StatusOK, o)
}

func (h *Handler) GetNetWorthStats(w http.ResponseWriter, r *http.Request) {
	from, err := parseDateParam(r.URL.Query().Get("from"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := parseDateEndParam(r.URL.Query().Get("to"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid to")
		return
	}
	g := StatsGranularity(r.URL.Query().Get("granularity"))
	if g != "" && !g.Valid() {
		core.Error(w, http.StatusBadRequest, "granularity must be day, week, or month")
		return
	}
	out, err := h.service.GetNetWorthSeries(r.Context(), NetWorthQuery{From: from, To: to, Granularity: g})
	if err != nil {
		core.InternalError(w, r, err, "Failed to compute net worth")
		return
	}
	core.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetCategoryStats(w http.ResponseWriter, r *http.Request) {
	t := txtype.Type(r.URL.Query().Get("type"))
	if !t.Valid() {
		core.Error(w, http.StatusBadRequest, "type must be income, expense, or transfer")
		return
	}
	from, err := parseDateParam(r.URL.Query().Get("from"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := parseDateEndParam(r.URL.Query().Get("to"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid to")
		return
	}
	accountID, err := parseOptionalIntParam(r.URL.Query().Get("account_id"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid account_id")
		return
	}
	out, err := h.service.GetCategoryStats(r.Context(), CategoryStatsQuery{
		Type: t, From: from, To: to, AccountID: accountID,
	})
	if err != nil {
		core.InternalError(w, r, err, "Failed to compute category stats")
		return
	}
	core.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetMonthlyStats(w http.ResponseWriter, r *http.Request) {
	from, err := parseDateParam(r.URL.Query().Get("from"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := parseDateEndParam(r.URL.Query().Get("to"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid to")
		return
	}
	accountID, err := parseOptionalIntParam(r.URL.Query().Get("account_id"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid account_id")
		return
	}
	categoryID, err := parseOptionalIntParam(r.URL.Query().Get("category_id"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "invalid category_id")
		return
	}
	out, err := h.service.GetMonthlyStats(r.Context(), MonthlyStatsQuery{
		From: from, To: to, AccountID: accountID, CategoryID: categoryID,
	})
	if err != nil {
		core.InternalError(w, r, err, "Failed to compute monthly stats")
		return
	}
	core.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetEstimation(w http.ResponseWriter, r *http.Request) {
	start, err := parseMonthParam(r.URL.Query().Get("start_month"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "start_month is required (YYYY-MM)")
		return
	}
	end, err := parseMonthParam(r.URL.Query().Get("end_month"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "end_month is required (YYYY-MM)")
		return
	}
	if end.Before(start) {
		core.Error(w, http.StatusBadRequest, "end_month must be on or after start_month")
		return
	}
	mode := EstimationMode(r.URL.Query().Get("mode"))
	if !mode.Valid() {
		core.Error(w, http.StatusBadRequest, "mode must be rate or saving")
		return
	}
	out, err := h.service.GetEstimation(r.Context(), EstimationQuery{
		StartMonth: start, EndMonth: end, Mode: mode,
	})
	if err != nil {
		core.InternalError(w, r, err, "Failed to compute estimation")
		return
	}
	core.JSON(w, http.StatusOK, out)
}
