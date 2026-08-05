package api

import (
	"net/http"
)

func (s *Server) periodAnalysis(w http.ResponseWriter, r *http.Request) {
	from, to, exact, ok := queryExactRange(w, r, 1, 366)
	if !ok {
		return
	}
	if !exact {
		writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "from/to 为必填参数")
		return
	}
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "day"
	}
	if bucket != "day" && bucket != "month" {
		writeError(w, http.StatusBadRequest, "INVALID_BUCKET", "bucket 只能为 day 或 month")
		return
	}
	analysis, err := s.store.PeriodAnalysis(r.Context(), currentUser(r).ID, from, to, bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "期间账本分析读取失败")
		return
	}
	writeData(w, analysis)
}
