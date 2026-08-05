package api

import (
	"net/http/httptest"
	"testing"
)

func TestQueryExactRangeAcceptsSingleDayPeriod(t *testing.T) {
	r := httptest.NewRequest("GET", "/?from=2026-07-20&to=2026-07-20", nil)
	from, to, exact, ok := queryExactRange(httptest.NewRecorder(), r, 1, 366)
	if !ok || !exact || from.Format("2006-01-02") != "2026-07-20" || to.Format("2006-01-02") != "2026-07-20" {
		t.Fatalf("queryExactRange() = %v, %v, %v, %v", from, to, exact, ok)
	}
}

func TestQueryExactRangeRejectsPartialAndMixedQueries(t *testing.T) {
	tests := []string{
		"/?from=2026-07-01",
		"/?to=2026-07-20",
		"/?from=2026-07-01&to=2026-07-20&windowDays=30",
		"/?from=2026-07-01&to=2026-07-20&asOf=2026-07-20",
	}
	for _, target := range tests {
		w := httptest.NewRecorder()
		_, _, _, ok := queryExactRange(w, httptest.NewRequest("GET", target, nil), 1, 366)
		if ok || w.Code != 400 {
			t.Fatalf("queryExactRange(%q) ok=%v status=%d, want false/400", target, ok, w.Code)
		}
	}
}

func TestQueryExactRangeRejectsPredictionShorterThanSevenDays(t *testing.T) {
	w := httptest.NewRecorder()
	_, _, _, ok := queryExactRange(w, httptest.NewRequest("GET", "/?from=2026-07-15&to=2026-07-20", nil), 7, 366)
	if ok || w.Code != 400 {
		t.Fatalf("queryExactRange() ok=%v status=%d, want false/400", ok, w.Code)
	}
}
