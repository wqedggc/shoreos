package api

import (
	"net/http"
	"time"

	"github.com/wqedggc/shoreos/internal/ledger"
)

func queryExactRange(w http.ResponseWriter, r *http.Request, minimumDays, maximumDays int) (time.Time, time.Time, bool, bool) {
	query := r.URL.Query()
	fromRaw, toRaw := query.Get("from"), query.Get("to")
	if fromRaw == "" && toRaw == "" {
		return time.Time{}, time.Time{}, false, true
	}
	if fromRaw == "" || toRaw == "" || query.Get("windowDays") != "" || query.Get("asOf") != "" {
		writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "from/to 必须同时提供，且不能与 windowDays/asOf 混用")
		return time.Time{}, time.Time{}, false, false
	}
	from, fromErr := time.ParseInLocation("2006-01-02", fromRaw, shoreLocation)
	to, toErr := time.ParseInLocation("2006-01-02", toRaw, shoreLocation)
	if fromErr != nil || toErr != nil {
		writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "from/to 必须为 YYYY-MM-DD")
		return time.Time{}, time.Time{}, false, false
	}
	if !ledger.ValidExactRange(from, to, minimumDays, maximumDays) {
		writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "日期范围长度不合法")
		return time.Time{}, time.Time{}, false, false
	}
	return from, to, true, true
}
