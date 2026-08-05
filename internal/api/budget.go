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
	"github.com/wqedggc/shoreos/internal/ledger"
	"github.com/wqedggc/shoreos/internal/repository/mysql"
)

var shoreLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func (s *Server) budgetPlan(w http.ResponseWriter, r *http.Request) {
	asOf, ok := queryAsOf(w, r)
	if !ok {
		return
	}
	plan, err := s.store.BudgetPlan(r.Context(), currentUser(r).ID, asOf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "预算计划读取失败")
		return
	}
	writeData(w, plan)
}

func (s *Server) saveBudgetPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlexibleBudgetMonthlyCents int64  `json:"flexibleBudgetMonthlyCents"`
		StartedOn                  string `json:"startedOn"`
		FixedExpenses              []struct {
			Name               string `json:"name"`
			CategoryGroup      string `json:"categoryGroup"`
			CategoryDetail     string `json:"categoryDetail"`
			MonthlyAmountCents int64  `json:"monthlyAmountCents"`
		} `json:"fixedExpenses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	startedOn, err := time.ParseInLocation("2006-01-02", req.StartedOn, shoreLocation)
	if err != nil || req.FlexibleBudgetMonthlyCents < 0 || len(req.FixedExpenses) > 100 {
		writeError(w, http.StatusBadRequest, "INVALID_BUDGET_PLAN", "预算金额、开始日期或固定支出不合法")
		return
	}
	input := mysql.SaveBudgetPlanInput{FlexibleBudgetMonthlyCents: req.FlexibleBudgetMonthlyCents, StartedOn: startedOn}
	for _, item := range req.FixedExpenses {
		item.Name = strings.TrimSpace(item.Name)
		item.CategoryGroup = strings.TrimSpace(item.CategoryGroup)
		item.CategoryDetail = strings.TrimSpace(item.CategoryDetail)
		if item.Name == "" || len(item.Name) > 128 || len(item.CategoryGroup) > 64 || len(item.CategoryDetail) > 64 || item.MonthlyAmountCents <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_FIXED_EXPENSE", "固定支出名称、分类或金额不合法")
			return
		}
		input.FixedExpenses = append(input.FixedExpenses, mysql.FixedExpense{
			Name: item.Name, CategoryGroup: item.CategoryGroup, CategoryDetail: item.CategoryDetail,
			MonthlyAmountCents: item.MonthlyAmountCents, Status: "active",
		})
	}
	if err := s.store.SaveBudgetPlan(r.Context(), currentUser(r).ID, input); err != nil {
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "预算计划保存失败")
		return
	}
	plan, err := s.store.BudgetPlan(r.Context(), currentUser(r).ID, time.Now().In(shoreLocation))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "预算保存后读取失败")
		return
	}
	writeData(w, plan)
}

func (s *Server) createSinkingFund(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeSinkingFund(w, r)
	if !ok {
		return
	}
	id, err := s.store.CreateSinkingFund(r.Context(), currentUser(r).ID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "准备金创建失败")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"id": id}})
}

func (s *Server) updateSinkingFund(w http.ResponseWriter, r *http.Request) {
	fundID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || fundID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "准备金编号不合法")
		return
	}
	input, ok := decodeSinkingFund(w, r)
	if !ok {
		return
	}
	if err := s.store.UpdateSinkingFund(r.Context(), currentUser(r).ID, fundID, input, time.Now().In(shoreLocation)); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "准备金不存在")
		case strings.Contains(err.Error(), "cannot be reactivated"):
			writeError(w, http.StatusConflict, "FUND_CANNOT_REACTIVATE", "已暂停或关闭的准备金不能重新启用，请新建用途池")
		default:
			writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "准备金保存失败")
		}
		return
	}
	writeData(w, map[string]string{"status": "ok"})
}

func decodeSinkingFund(w http.ResponseWriter, r *http.Request) (mysql.SaveSinkingFundInput, bool) {
	var req struct {
		Name                     string `json:"name"`
		MonthlyContributionCents int64  `json:"monthlyContributionCents"`
		OpeningBalanceCents      int64  `json:"openingBalanceCents"`
		StartedOn                string `json:"startedOn"`
		Status                   string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return mysql.SaveSinkingFundInput{}, false
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Status == "" {
		req.Status = "active"
	}
	startedOn, err := time.ParseInLocation("2006-01-02", req.StartedOn, shoreLocation)
	if err != nil || req.Name == "" || len(req.Name) > 128 || req.MonthlyContributionCents < 0 || req.OpeningBalanceCents < 0 || !oneOf(req.Status, "active", "paused", "closed") {
		writeError(w, http.StatusBadRequest, "INVALID_SINKING_FUND", "准备金名称、金额、日期或状态不合法")
		return mysql.SaveSinkingFundInput{}, false
	}
	return mysql.SaveSinkingFundInput{
		Name: req.Name, MonthlyContributionCents: req.MonthlyContributionCents,
		OpeningBalanceCents: req.OpeningBalanceCents, StartedOn: startedOn, Status: req.Status,
	}, true
}

func (s *Server) spendingBaseline(w http.ResponseWriter, r *http.Request) {
	from, to, exact, ok := queryExactRange(w, r, 7, 366)
	if !ok {
		return
	}
	if exact {
		baseline, err := s.store.SpendingBaselineRange(r.Context(), currentUser(r).ID, from, to)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "滚动支出基线读取失败")
			return
		}
		writeData(w, baseline)
		return
	}
	windowDays := ledger.DefaultWindowDays
	if raw := r.URL.Query().Get("windowDays"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || !ledger.ValidWindowDays(parsed) {
			writeError(w, http.StatusBadRequest, "INVALID_WINDOW", "windowDays 只能为 7、30、90 或 365")
			return
		}
		windowDays = parsed
	}
	asOf, ok := queryAsOf(w, r)
	if !ok {
		return
	}
	baseline, err := s.store.SpendingBaseline(r.Context(), currentUser(r).ID, asOf, windowDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "滚动支出基线读取失败")
		return
	}
	writeData(w, baseline)
}

func queryAsOf(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	raw := r.URL.Query().Get("asOf")
	if raw == "" {
		return time.Now().In(shoreLocation), true
	}
	asOf, err := time.ParseInLocation("2006-01-02", raw, shoreLocation)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_DATE", "asOf 必须为 YYYY-MM-DD")
		return time.Time{}, false
	}
	return asOf, true
}
