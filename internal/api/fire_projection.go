package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wqedggc/shoreos/internal/fire"
	"github.com/wqedggc/shoreos/internal/ledger"
	"github.com/wqedggc/shoreos/internal/repository/mysql"
)

type fireProjectionBundle struct {
	ScenarioID  string                        `json:"scenarioId"`
	Baseline    mysql.RollingSpendingBaseline `json:"spendingBaseline"`
	Model       fire.ScenarioModel            `json:"model"`
	TargetLine  fire.Projection               `json:"targetLine"`
	ActualLine  fire.Projection               `json:"actualLine"`
	DeltaMonths *int                          `json:"deltaMonths"`
}

type exceptionalImpact struct {
	HumanTransactionID          int64  `json:"humanTransactionId"`
	HumanTitle                  string `json:"humanTitle"`
	AmountCents                 int64  `json:"amountCents"`
	CounterfactualMonthsEarlier *int   `json:"counterfactualMonthsEarlier"`
}

func (s *Server) createFireProjectionRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScenarioID   string `json:"scenarioId"`
		SpendingMode string `json:"spendingMode"`
		WindowDays   int    `json:"windowDays"`
		AsOf         string `json:"asOf"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	req.ScenarioID = strings.TrimSpace(req.ScenarioID)
	if req.SpendingMode == "" {
		req.SpendingMode = "ledger"
	}
	if req.WindowDays == 0 {
		req.WindowDays = ledger.DefaultWindowDays
	}
	if req.ScenarioID == "" || !oneOf(req.SpendingMode, "manual", "ledger") || !ledger.ValidWindowDays(req.WindowDays) {
		writeError(w, http.StatusBadRequest, "INVALID_PROJECTION_INPUT", "FIRE 情景、支出模式或观察窗口不合法")
		return
	}
	asOf, ok := bodyAsOf(w, req.AsOf)
	if !ok {
		return
	}
	scenario, bundle, err := s.fireProjectionBundle(r, req.ScenarioID, req.SpendingMode, req.WindowDays, asOf)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "FIRE 情景不存在")
		} else {
			writeError(w, http.StatusInternalServerError, "PROJECTION_FAILED", "FIRE 测算失败")
		}
		return
	}
	snapshot := map[string]any{
		"scenarioId": req.ScenarioID, "spendingMode": req.SpendingMode, "windowDays": req.WindowDays,
		"asOf": asOf.Format("2006-01-02"), "scenario": scenario.Profile, "result": bundle,
	}
	run, err := s.store.CreateFireProjectionRun(r.Context(), currentUser(r).ID, scenario.ID, req.SpendingMode, snapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "FIRE 测算结果保存失败")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"run": run, "projection": bundle}})
}

func (s *Server) fireProjectionRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || runID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "测算编号不合法")
		return
	}
	run, err := s.store.FireProjectionRun(r.Context(), currentUser(r).ID, runID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "测算记录不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "测算记录读取失败")
		return
	}
	writeData(w, run)
}

func (s *Server) spendingImpact(w http.ResponseWriter, r *http.Request) {
	scenarioID := strings.TrimSpace(chi.URLParam(r, "scenarioId"))
	from, exactTo, exact, ok := queryExactRange(w, r, 7, 366)
	if !ok {
		return
	}
	windowDays := ledger.DefaultWindowDays
	if !exact {
		if raw := r.URL.Query().Get("windowDays"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || !ledger.ValidWindowDays(parsed) {
				writeError(w, http.StatusBadRequest, "INVALID_WINDOW", "windowDays 只能为 7、30、90 或 365")
				return
			}
			windowDays = parsed
		}
	}
	asOf := exactTo
	if !exact {
		asOf, ok = queryAsOf(w, r)
		if !ok {
			return
		}
		from = asOf.AddDate(0, 0, -(windowDays - 1))
	}
	var bundle fireProjectionBundle
	var err error
	if exact {
		_, bundle, err = s.fireProjectionBundleRange(r, scenarioID, from, asOf)
	} else {
		_, bundle, err = s.fireProjectionBundle(r, scenarioID, "ledger", windowDays, asOf)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "FIRE 情景不存在")
		} else {
			writeError(w, http.StatusInternalServerError, "PROJECTION_FAILED", "支出影响计算失败")
		}
		return
	}
	categories, err := s.store.SpendingImpactCategories(r.Context(), currentUser(r).ID, from, asOf, 5)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "支出分类影响读取失败")
		return
	}
	transactions, err := s.store.SpendingImpactTransactions(r.Context(), currentUser(r).ID, from, asOf, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "支出交易影响读取失败")
		return
	}
	exceptional := make([]exceptionalImpact, 0)
	for _, item := range transactions {
		if item.BudgetTreatment != string(ledger.BudgetExceptional) {
			continue
		}
		counterfactual := fire.CalculateWithAdditionalAssets(bundle.Model, bundle.ActualLine.AnnualExpenseCents, item.AmountCents, asOf)
		exceptional = append(exceptional, exceptionalImpact{
			HumanTransactionID: item.HumanTransactionID, HumanTitle: item.HumanTitle, AmountCents: item.AmountCents,
			CounterfactualMonthsEarlier: monthsEarlier(bundle.ActualLine.MonthsToFire, counterfactual.MonthsToFire),
		})
	}
	plan, err := s.store.BudgetPlan(r.Context(), currentUser(r).ID, asOf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "准备金状态读取失败")
		return
	}
	writeData(w, map[string]any{
		"projection": bundle, "topCategories": categories, "topTransactions": transactions,
		"exceptionalImpacts": exceptional, "sinkingFunds": plan.SinkingFunds,
	})
}

func (s *Server) fireProjectionBundle(r *http.Request, scenarioUID, spendingMode string, windowDays int, asOf time.Time) (mysql.FireScenario, fireProjectionBundle, error) {
	scenario, err := s.store.FireScenarioByUID(r.Context(), currentUser(r).ID, scenarioUID)
	if err != nil {
		return mysql.FireScenario{}, fireProjectionBundle{}, err
	}
	baseline, err := s.store.SpendingBaseline(r.Context(), currentUser(r).ID, asOf, windowDays)
	if err != nil {
		return mysql.FireScenario{}, fireProjectionBundle{}, err
	}
	return s.fireProjectionBundleFromBaseline(r, scenario, spendingMode, asOf, baseline)
}

func (s *Server) fireProjectionBundleRange(r *http.Request, scenarioUID string, from, to time.Time) (mysql.FireScenario, fireProjectionBundle, error) {
	scenario, err := s.store.FireScenarioByUID(r.Context(), currentUser(r).ID, scenarioUID)
	if err != nil {
		return mysql.FireScenario{}, fireProjectionBundle{}, err
	}
	baseline, err := s.store.SpendingBaselineRange(r.Context(), currentUser(r).ID, from, to)
	if err != nil {
		return mysql.FireScenario{}, fireProjectionBundle{}, err
	}
	return s.fireProjectionBundleFromBaseline(r, scenario, "ledger", to, baseline)
}

func (s *Server) fireProjectionBundleFromBaseline(r *http.Request, scenario mysql.FireScenario, spendingMode string, asOf time.Time, baseline mysql.RollingSpendingBaseline) (mysql.FireScenario, fireProjectionBundle, error) {
	plan, err := s.store.BudgetPlan(r.Context(), currentUser(r).ID, asOf)
	if err != nil {
		return mysql.FireScenario{}, fireProjectionBundle{}, err
	}
	var reserved int64
	for _, fund := range plan.SinkingFunds {
		if fund.CurrentBalanceCents > 0 {
			reserved += fund.CurrentBalanceCents
		}
	}
	model := fire.ModelFromProfile(scenario.Profile, reserved)
	targetAnnual := model.ManualAnnualExpenseCents
	actualAnnual := model.ManualAnnualExpenseCents
	if spendingMode == "ledger" {
		targetAnnual = baseline.TargetAnnualExpenseCents
		actualAnnual = baseline.ActualPaceAnnualExpenseCents
	}
	targetLine := fire.Calculate(model, targetAnnual, asOf)
	actualLine := fire.Calculate(model, actualAnnual, asOf)
	if spendingMode == "ledger" && !plan.Configured {
		targetLine = fire.Projection{
			AnnualExpenseCents: targetAnnual, FireTargetCents: model.FireTargetCents,
			InvestableAssetCents: model.InvestableAssetCents,
			GapCents:             projectionGap(model.FireTargetCents, model.InvestableAssetCents),
		}
	}
	bundle := fireProjectionBundle{
		ScenarioID: scenario.ProfileUID, Baseline: baseline, Model: model,
		TargetLine: targetLine, ActualLine: actualLine,
		DeltaMonths: monthDelta(targetLine.MonthsToFire, actualLine.MonthsToFire),
	}
	return scenario, bundle, nil
}

func projectionGap(target, assets int64) int64 {
	if target > assets {
		return target - assets
	}
	return 0
}

func monthDelta(target, actual *int) *int {
	if target == nil || actual == nil {
		return nil
	}
	delta := *actual - *target
	return &delta
}

func monthsEarlier(actual, counterfactual *int) *int {
	if actual == nil || counterfactual == nil {
		return nil
	}
	delta := *actual - *counterfactual
	if delta < 0 {
		delta = 0
	}
	return &delta
}

func bodyAsOf(w http.ResponseWriter, raw string) (time.Time, bool) {
	if raw == "" {
		return time.Now().In(shoreLocation), true
	}
	value, err := time.ParseInLocation("2006-01-02", raw, shoreLocation)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_DATE", "asOf 必须为 YYYY-MM-DD")
		return time.Time{}, false
	}
	return value, true
}
