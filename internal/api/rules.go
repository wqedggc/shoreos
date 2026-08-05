package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wqedggc/shoreos/internal/ledger"
	"github.com/wqedggc/shoreos/internal/repository/mysql"
)

type classificationRuleRequest struct {
	MatchField      string `json:"matchField"`
	MatchValue      string `json:"matchValue"`
	CategoryGroup   string `json:"categoryGroup"`
	CategoryDetail  string `json:"categoryDetail"`
	Purpose         string `json:"purpose"`
	Necessity       string `json:"necessity"`
	Planning        string `json:"planning"`
	Recurrence      string `json:"recurrence"`
	BudgetTreatment string `json:"budgetTreatment"`
	SinkingFundID   *int64 `json:"sinkingFundId"`
}

func (s *Server) classificationRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ClassificationRules(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "分类规则读取失败")
		return
	}
	writeData(w, map[string]any{"rules": rules})
}

func (s *Server) previewClassificationRule(w http.ResponseWriter, r *http.Request) {
	var req classificationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	normalizeRuleRequest(&req)
	if !validRuleMatch(req.MatchField, req.MatchValue) {
		writeError(w, http.StatusBadRequest, "INVALID_RULE", "规则匹配字段或匹配值不合法")
		return
	}
	preview, err := s.store.PreviewClassificationRule(r.Context(), currentUser(r).ID, req.MatchField, req.MatchValue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "规则命中预览失败")
		return
	}
	writeData(w, preview)
}

func (s *Server) createClassificationRule(w http.ResponseWriter, r *http.Request) {
	var req classificationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	normalizeRuleRequest(&req)
	if !validRuleMatch(req.MatchField, req.MatchValue) || !validRuleAssignments(req) {
		writeError(w, http.StatusBadRequest, "INVALID_RULE", "规则匹配条件或解释赋值不合法")
		return
	}
	rule, preview, created, err := s.store.CreateClassificationRule(r.Context(), currentUser(r).ID, mysql.ClassificationRule{
		MatchField: req.MatchField, MatchValue: req.MatchValue, CategoryGroup: req.CategoryGroup,
		CategoryDetail: req.CategoryDetail, Purpose: req.Purpose, Necessity: req.Necessity,
		Planning: req.Planning, Recurrence: req.Recurrence, BudgetTreatment: req.BudgetTreatment,
		SinkingFundID: req.SinkingFundID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "INVALID_SINKING_FUND", "准备金不属于当前用户")
		} else {
			writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "分类规则保存失败")
		}
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"data": map[string]any{"rule": rule, "preview": preview, "created": created}})
}

func (s *Server) setClassificationRuleEnabled(w http.ResponseWriter, r *http.Request) {
	ruleID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || ruleID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "规则编号不合法")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "enabled 必须为布尔值")
		return
	}
	if err := s.store.SetClassificationRuleEnabled(r.Context(), currentUser(r).ID, ruleID, *req.Enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "分类规则不存在")
		} else {
			writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "分类规则状态保存失败")
		}
		return
	}
	writeData(w, map[string]any{"enabled": *req.Enabled})
}

func normalizeRuleRequest(req *classificationRuleRequest) {
	req.MatchField = strings.TrimSpace(req.MatchField)
	req.MatchValue = strings.TrimSpace(req.MatchValue)
	req.CategoryGroup = strings.TrimSpace(req.CategoryGroup)
	req.CategoryDetail = strings.TrimSpace(req.CategoryDetail)
	req.Purpose = strings.TrimSpace(req.Purpose)
	req.Necessity = strings.TrimSpace(req.Necessity)
	req.Planning = strings.TrimSpace(req.Planning)
	req.Recurrence = strings.TrimSpace(req.Recurrence)
	req.BudgetTreatment = strings.TrimSpace(req.BudgetTreatment)
}

func validRuleMatch(field, value string) bool {
	return oneOf(field, "exact_merchant", "product_keyword", "source_category") && value != "" && len(value) <= 255
}

func validRuleAssignments(req classificationRuleRequest) bool {
	hasAssignment := req.CategoryGroup != "" || req.CategoryDetail != "" || req.Purpose != "" || req.Necessity != "" ||
		req.Planning != "" || req.Recurrence != "" || req.BudgetTreatment != ""
	return hasAssignment && len(req.CategoryGroup) <= 64 && len(req.CategoryDetail) <= 64 &&
		oneOf(req.Purpose, "", "生存与责任", "运行与维护", "成长与能力", "恢复与体验", "资产配置", "关系与给予", "未知") &&
		oneOf(req.Necessity, "", "必需", "重要可选", "可选", "未知") &&
		oneOf(req.Planning, "", "计划内", "计划外", "未知") &&
		oneOf(req.Recurrence, "", "一次性", "周期性", "不规律重复", "未知") &&
		(req.BudgetTreatment == "" || ledger.ValidBudgetTreatment(req.BudgetTreatment)) &&
		(req.BudgetTreatment != string(ledger.BudgetFundedIrregular) || req.SinkingFundID != nil)
}
