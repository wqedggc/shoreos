package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/wqedggc/shoreos/internal/auth"
	"github.com/wqedggc/shoreos/internal/config"
	identityauth "github.com/wqedggc/shoreos/internal/identity"
	"github.com/wqedggc/shoreos/internal/ledger"
	"github.com/wqedggc/shoreos/internal/repository/mysql"
	"github.com/wqedggc/shoreos/pkg/identity"
	"github.com/wqedggc/shoreos/web"
)

type Server struct {
	store            *mysql.Store
	static           fs.FS
	ledgerStorage    *ledger.Storage
	ledgerWorker     ledger.Worker
	config           config.Config
	wechat           identityauth.WechatExchanger
	loginThrottle    *auth.LoginThrottle
	registerThrottle *auth.LoginThrottle
}

type ctxKey string

const userKey ctxKey = "user"

func NewServer(store *mysql.Store, cfg config.Config) (*Server, error) {
	static, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return nil, err
	}
	ledgerStorage, err := ledger.NewStorage(cfg.LedgerStorageDir)
	if err != nil {
		return nil, err
	}
	return &Server{
		store:            store,
		static:           static,
		ledgerStorage:    ledgerStorage,
		ledgerWorker:     ledger.Worker{PythonBin: cfg.LedgerPythonBin, Script: cfg.LedgerWorkerPath},
		config:           cfg,
		wechat:           identityauth.WechatExchanger{AppID: cfg.WechatAppID, Secret: cfg.WechatSecret},
		loginThrottle:    auth.NewLoginThrottle(cfg.LoginMaxFailures, cfg.LoginFailureWindow),
		registerThrottle: auth.NewLoginThrottle(cfg.LoginMaxFailures, cfg.LoginFailureWindow),
	}, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(limitRequestBody)

	r.Get("/healthz", s.health)
	r.Get("/readyz", s.ready)

	r.Route("/api/v1", func(r chi.Router) {
		if s.config.EnableLocalBootstrap {
			r.Post("/auth/bootstrap", s.bootstrap)
		}
		r.Post("/auth/login", s.login)
		if s.config.EnableRegistration {
			r.Post("/auth/register", s.register)
		}
		r.Post("/auth/wechat/login", s.wechatLogin)
		r.Post("/auth/wechat/bind", s.wechatBind)

		r.Group(func(r chi.Router) {
			r.Use(s.auth)
			r.Post("/auth/logout", s.logout)
			r.Post("/auth/wechat/bind-codes", s.createWechatBindingCode)
			r.Get("/me", s.me)
			r.Patch("/me", s.updateMe)
			r.Get("/fire/scenarios", s.getProfiles)
			r.Put("/fire/scenarios/sync", s.syncProfiles)
			r.Post("/fire/scenarios", s.syncProfiles)
			r.Get("/fire/asset-snapshots", s.emptyList)
			r.Post("/fire/asset-snapshots", s.echoCreated)
			r.Post("/fire/projection-runs", s.createFireProjectionRun)
			r.Get("/fire/projection-runs/{id}", s.fireProjectionRun)
			r.Get("/fire/scenarios/{scenarioId}/spending-impact", s.spendingImpact)
			r.Get("/fire/summary", s.getProfiles)
			r.Post("/ledger/imports", s.importLedger)
			r.Get("/ledger/imports", s.ledgerImports)
			r.Post("/ledger/materialize", s.materializeLedger)
			r.Get("/ledger/transactions", s.ledgerTransactions)
			r.Get("/ledger/transactions/{id}", s.ledgerTransaction)
			r.Get("/ledger/human-transactions", s.humanTransactions)
			r.Get("/ledger/human-transactions/{id}", s.humanTransaction)
			r.Patch("/ledger/human-transactions/{id}/interpretation", s.updateHumanTransactionInterpretation)
			r.Get("/ledger/budget-plan", s.budgetPlan)
			r.Put("/ledger/budget-plan", s.saveBudgetPlan)
			r.Post("/ledger/sinking-funds", s.createSinkingFund)
			r.Patch("/ledger/sinking-funds/{id}", s.updateSinkingFund)
			r.Get("/ledger/spending-baseline", s.spendingBaseline)
			r.Get("/ledger/period-analysis", s.periodAnalysis)
			r.Get("/ledger/classification-rules", s.classificationRules)
			r.Post("/ledger/classification-rules/preview", s.previewClassificationRule)
			r.Post("/ledger/classification-rules", s.createClassificationRule)
			r.Patch("/ledger/classification-rules/{id}", s.setClassificationRuleEnabled)
			r.Get("/ledger/link-candidates", s.ledgerLinkCandidates)
			r.Post("/ledger/link-candidates/{id}/decision", s.decideLedgerLinkCandidate)
		})
	})

	fileServer := http.FileServer(http.FS(s.static))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, s.static, "index.html")
	})
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	})
	return r
}

func (s *Server) importLedger(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_UPLOAD", "账单文件无法读取或超过 50MB")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "FILE_REQUIRED", "请上传账单文件")
		return
	}
	defer file.Close()

	stored, err := s.ledgerStorage.Save(currentUser(r).ID, header.Filename, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UPLOAD_FAILED", "账单文件保存失败")
		return
	}
	parsed, err := s.ledgerWorker.Parse(r.Context(), stored.Path)
	if err != nil {
		_ = s.ledgerStorage.Remove(stored)
		log.Printf("ledger parser user=%d: %v", currentUser(r).ID, err)
		if strings.Contains(err.Error(), "缺少 openpyxl") || strings.Contains(err.Error(), "缺少 pypdf") {
			writeError(w, http.StatusServiceUnavailable, "PARSER_UNAVAILABLE", "账单解析环境未安装完整，请联系管理员")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "PARSE_FAILED", "文件内容不是当前支持的支付宝、微信或招商官方账单格式")
		return
	}
	imported, err := s.store.ImportLedger(r.Context(), currentUser(r).ID, stored.Key, parsed)
	if err != nil {
		_ = s.ledgerStorage.Remove(stored)
		writeError(w, http.StatusInternalServerError, "IMPORT_FAILED", "账单导入失败")
		return
	}
	if _, err := s.store.MaterializeLedger(r.Context(), currentUser(r).ID); err != nil {
		if imported.AlreadyImported || imported.Reprocessed {
			_ = s.ledgerStorage.Remove(stored)
		}
		writeError(w, http.StatusInternalServerError, "MATERIALIZE_FAILED", "账单已保存，但人类交易生成失败")
		return
	}
	if imported.AlreadyImported || imported.Reprocessed {
		_ = s.ledgerStorage.Remove(stored)
		writeData(w, imported)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": imported})
}

func (s *Server) materializeLedger(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.MaterializeLedger(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MATERIALIZE_FAILED", "人类交易生成失败")
		return
	}
	writeData(w, result)
}

func (s *Server) ledgerImports(w http.ResponseWriter, r *http.Request) {
	imports, err := s.store.LedgerImports(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "账单导入记录读取失败")
		return
	}
	writeData(w, map[string]any{"imports": imports})
}

func (s *Server) ledgerTransactions(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit 必须在 1 到 100 之间")
			return
		}
		limit = parsed
	}
	transactions, err := s.store.LedgerTransactions(r.Context(), currentUser(r).ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "账单事实读取失败")
		return
	}
	writeData(w, map[string]any{"transactions": transactions})
}

func (s *Server) ledgerTransaction(w http.ResponseWriter, r *http.Request) {
	transactionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || transactionID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "账单编号不合法")
		return
	}
	transaction, err := s.store.LedgerTransaction(r.Context(), currentUser(r).ID, transactionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "账单不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "账单详情读取失败")
		return
	}
	writeData(w, transaction)
}

func (s *Server) humanTransactions(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(w, r)
	if !ok {
		return
	}
	filter := mysql.HumanTransactionFilter{
		Limit:           limit,
		From:            strings.TrimSpace(r.URL.Query().Get("from")),
		To:              strings.TrimSpace(r.URL.Query().Get("to")),
		SourcePlatform:  strings.TrimSpace(r.URL.Query().Get("source")),
		TransactionType: strings.TrimSpace(r.URL.Query().Get("type")),
		CategoryGroup:   strings.TrimSpace(r.URL.Query().Get("categoryGroup")),
		CategoryDetail:  strings.TrimSpace(r.URL.Query().Get("categoryDetail")),
		Purpose:         strings.TrimSpace(r.URL.Query().Get("purpose")),
		Necessity:       strings.TrimSpace(r.URL.Query().Get("necessity")),
		Planning:        strings.TrimSpace(r.URL.Query().Get("planning")),
		Recurrence:      strings.TrimSpace(r.URL.Query().Get("recurrence")),
		BudgetTreatment: strings.TrimSpace(r.URL.Query().Get("budgetTreatment")),
	}
	if filter.From != "" {
		if _, err := time.Parse("2006-01-02", filter.From); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_DATE", "from 必须为 YYYY-MM-DD")
			return
		}
	}
	if filter.To != "" {
		if _, err := time.Parse("2006-01-02", filter.To); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_DATE", "to 必须为 YYYY-MM-DD")
			return
		}
	}
	if raw := r.URL.Query().Get("needsReview"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_FILTER", "needsReview 必须为 true 或 false")
			return
		}
		filter.NeedsReview = &parsed
	}
	transactions, err := s.store.HumanTransactions(r.Context(), currentUser(r).ID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "人类交易读取失败")
		return
	}
	writeData(w, map[string]any{"transactions": transactions})
}

func (s *Server) humanTransaction(w http.ResponseWriter, r *http.Request) {
	humanID, ok := pathID(w, r)
	if !ok {
		return
	}
	detail, err := s.store.HumanTransaction(r.Context(), currentUser(r).ID, humanID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "人类交易不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "人类交易详情读取失败")
		return
	}
	writeData(w, detail)
}

func (s *Server) updateHumanTransactionInterpretation(w http.ResponseWriter, r *http.Request) {
	humanID, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		HumanTitle      *string `json:"humanTitle"`
		UserNote        *string `json:"userNote"`
		CategoryGroup   *string `json:"categoryGroup"`
		CategoryDetail  *string `json:"categoryDetail"`
		Purpose         *string `json:"purpose"`
		Necessity       *string `json:"necessity"`
		Planning        *string `json:"planning"`
		Recurrence      *string `json:"recurrence"`
		BudgetTreatment *string `json:"budgetTreatment"`
		SinkingFundID   *int64  `json:"sinkingFundId"`
		Confidence      *string `json:"confidence"`
		EvidenceBasis   *string `json:"evidenceBasis"`
		DecisionScope   *string `json:"decisionScope"`
		Rationale       *string `json:"rationale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	current, err := s.store.HumanTransaction(r.Context(), currentUser(r).ID, humanID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "人类交易不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "人类交易读取失败")
		return
	}
	override := mysql.HumanTransactionOverride{
		HumanTitle: current.HumanTitle, UserNote: current.UserNote, CategoryGroup: current.CategoryGroup,
		CategoryDetail: current.CategoryDetail, Purpose: current.Purpose, Necessity: current.Necessity,
		Planning: current.Planning, Recurrence: current.Recurrence, BudgetTreatment: current.BudgetTreatment,
		SinkingFundID: current.SinkingFundID, Confidence: current.Confidence, EvidenceBasis: current.EvidenceBasis,
		DecisionScope: current.DecisionScope, Rationale: current.Rationale,
	}
	if req.HumanTitle != nil {
		override.HumanTitle = strings.TrimSpace(*req.HumanTitle)
	}
	if req.UserNote != nil {
		override.UserNote = strings.TrimSpace(*req.UserNote)
	}
	if req.CategoryGroup != nil {
		override.CategoryGroup = strings.TrimSpace(*req.CategoryGroup)
	}
	if req.CategoryDetail != nil {
		override.CategoryDetail = strings.TrimSpace(*req.CategoryDetail)
	}
	if req.Purpose != nil {
		override.Purpose = strings.TrimSpace(*req.Purpose)
	}
	if req.Necessity != nil {
		override.Necessity = strings.TrimSpace(*req.Necessity)
	}
	if req.Planning != nil {
		override.Planning = strings.TrimSpace(*req.Planning)
	}
	if req.Recurrence != nil {
		override.Recurrence = strings.TrimSpace(*req.Recurrence)
	}
	if req.BudgetTreatment != nil {
		override.BudgetTreatment = strings.TrimSpace(*req.BudgetTreatment)
	}
	if req.SinkingFundID != nil {
		override.SinkingFundID = req.SinkingFundID
	}
	if req.Confidence != nil {
		override.Confidence = strings.TrimSpace(*req.Confidence)
	}
	if req.EvidenceBasis != nil {
		override.EvidenceBasis = strings.TrimSpace(*req.EvidenceBasis)
	}
	if req.DecisionScope != nil {
		override.DecisionScope = strings.TrimSpace(*req.DecisionScope)
	}
	if req.Rationale != nil {
		override.Rationale = strings.TrimSpace(*req.Rationale)
	}
	if override.BudgetTreatment != string(ledger.BudgetFundedIrregular) {
		override.SinkingFundID = nil
	}
	if !validInterpretation(override) {
		writeError(w, http.StatusBadRequest, "INVALID_INTERPRETATION", "五维解释值不合法")
		return
	}
	if err := s.store.UpdateHumanTransactionOverride(r.Context(), currentUser(r).ID, humanID, override); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "人类交易不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "五维解释保存失败")
		return
	}
	updated, err := s.store.HumanTransaction(r.Context(), currentUser(r).ID, humanID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "保存后读取失败")
		return
	}
	writeData(w, updated)
}

func (s *Server) ledgerLinkCandidates(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(w, r)
	if !ok {
		return
	}
	candidates, err := s.store.LedgerLinkCandidates(r.Context(), currentUser(r).ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "关联候选读取失败")
		return
	}
	writeData(w, map[string]any{"candidates": candidates})
}

func (s *Server) decideLedgerLinkCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Decision != "confirmed" && req.Decision != "rejected") {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "decision 必须为 confirmed 或 rejected")
		return
	}
	if err := s.store.DecideLedgerLinkCandidate(r.Context(), currentUser(r).ID, candidateID, req.Decision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "关联候选不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "关联候选保存失败")
		return
	}
	writeData(w, map[string]string{"status": req.Decision})
}

func queryLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit 必须在 1 到 1000 之间")
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "编号不合法")
		return 0, false
	}
	return id, true
}

func validInterpretation(override mysql.HumanTransactionOverride) bool {
	return len(override.HumanTitle) <= 255 && len(override.UserNote) <= 1000 && len(override.Rationale) <= 1000 && len(override.CategoryGroup) <= 64 && len(override.CategoryDetail) <= 64 &&
		oneOf(override.Purpose, "", "生存与责任", "运行与维护", "成长与能力", "恢复与体验", "资产配置", "关系与给予", "未知") &&
		oneOf(override.Necessity, "", "必需", "重要可选", "可选", "未知") &&
		oneOf(override.Planning, "", "计划内", "计划外", "未知") &&
		oneOf(override.Recurrence, "", "一次性", "周期性", "不规律重复", "未知") &&
		ledger.ValidBudgetTreatment(override.BudgetTreatment) &&
		(override.BudgetTreatment != string(ledger.BudgetFundedIrregular) || override.SinkingFundID != nil) &&
		oneOf(override.Confidence, "confirmed", "tentative", "unknown") &&
		oneOf(override.EvidenceBasis, "source_link", "source_text", "user_memory", "insufficient") &&
		oneOf(override.DecisionScope, "single_transaction", "rule_candidate")
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DB_NOT_READY", "数据库不可用")
		return
	}
	writeData(w, map[string]string{"status": "ready"})
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	if !isLocalBootstrapRequest(r) {
		writeError(w, http.StatusForbidden, "LOCAL_ONLY", "本机初始化只能从本机访问")
		return
	}
	user, token, err := s.store.Bootstrap(r.Context())
	if err != nil {
		if errors.Is(err, mysql.ErrAlreadyInitialized) {
			writeError(w, http.StatusConflict, "ALREADY_INITIALIZED", "ShoreOS 已初始化，请使用登录接口")
			return
		}
		log.Printf("bootstrap: %v", err)
		writeError(w, http.StatusInternalServerError, "BOOTSTRAP_FAILED", "初始化用户失败")
		return
	}
	writeData(w, map[string]any{"token": token, "user": user})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if !s.loginThrottle.Allow(req.Username) {
		w.Header().Set("Retry-After", strconv.Itoa(int(s.config.LoginFailureWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "LOGIN_THROTTLED", "登录尝试过多，请稍后再试")
		return
	}
	user, token, err := s.store.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		s.loginThrottle.RecordFailure(req.Username)
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码错误")
		return
	}
	s.loginThrottle.Reset(req.Username)
	writeData(w, map[string]any{"token": token, "user": user})
}

var registrationUsernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	username, validationCode, validationMessage := validateRegistration(req.Username, req.Password)
	if validationCode != "" {
		writeError(w, http.StatusBadRequest, validationCode, validationMessage)
		return
	}
	throttleKey := registrationRemoteKey(r)
	if !s.registerThrottle.Allow(throttleKey) {
		w.Header().Set("Retry-After", strconv.Itoa(int(s.config.LoginFailureWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "REGISTRATION_THROTTLED", "注册尝试过多，请稍后再试")
		return
	}
	s.registerThrottle.RecordAttempt(throttleKey)
	user, token, err := s.store.Register(r.Context(), username, req.Password)
	if err != nil {
		if errors.Is(err, mysql.ErrUsernameTaken) {
			writeError(w, http.StatusConflict, "USERNAME_TAKEN", "用户名已被使用")
			return
		}
		log.Printf("register: %v", err)
		writeError(w, http.StatusInternalServerError, "REGISTRATION_FAILED", "注册失败，请稍后再试")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"token": token, "user": user}})
}

func validateRegistration(rawUsername, password string) (username, code, message string) {
	username = strings.ToLower(strings.TrimSpace(rawUsername))
	if !registrationUsernamePattern.MatchString(username) {
		return "", "INVALID_USERNAME", "用户名需为 3–32 位小写字母、数字、点、下划线或短横线，且必须以字母或数字开头"
	}
	passwordBytes := len([]byte(password))
	if passwordBytes < 10 || passwordBytes > 72 {
		return "", "INVALID_PASSWORD", "密码长度需为 10–72 个字节"
	}
	return username, "", ""
}

func registrationRemoteKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

const maxRequestBodyBytes int64 = 50 << 20

func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) wechatLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "微信登录 code 不能为空")
		return
	}
	session, err := s.wechat.Exchange(r.Context(), req.Code)
	if err != nil {
		writeError(w, http.StatusBadGateway, "WECHAT_LOGIN_FAILED", "微信登录暂不可用")
		return
	}
	user, err := s.store.UserByIdentity(r.Context(), identity.ProviderWechatMini, session.OpenID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusForbidden, "WECHAT_NOT_BOUND", "微信尚未绑定 ShoreOS 用户")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUTH_FAILED", "微信身份读取失败")
		return
	}
	s.writeSessionResponse(w, r, user)
}

func (s *Server) wechatBind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code        string `json:"code"`
		BindingCode string `json:"bindingCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.BindingCode) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "微信 code 和绑定码不能为空")
		return
	}
	session, err := s.wechat.Exchange(r.Context(), req.Code)
	if err != nil {
		writeError(w, http.StatusBadGateway, "WECHAT_LOGIN_FAILED", "微信登录暂不可用")
		return
	}
	user, err := s.store.ConsumeIdentityBindingCode(r.Context(), req.BindingCode, identity.ProviderWechatMini, session.OpenID, session.UnionID)
	switch {
	case errors.Is(err, mysql.ErrBindingCodeInvalid), errors.Is(err, mysql.ErrBindingCodeExpired):
		writeError(w, http.StatusForbidden, "BINDING_CODE_INVALID", "绑定码无效或已过期")
		return
	case errors.Is(err, mysql.ErrIdentityBound):
		writeError(w, http.StatusConflict, "WECHAT_ALREADY_BOUND", "该微信已绑定其他 ShoreOS 用户")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "BIND_FAILED", "微信绑定失败")
		return
	}
	s.writeSessionResponse(w, r, user)
}

func (s *Server) createWechatBindingCode(w http.ResponseWriter, r *http.Request) {
	code, expiresAt, err := s.store.CreateIdentityBindingCode(r.Context(), currentUser(r).ID, identity.ProviderWechatMini, s.config.IdentityBindTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "BINDING_CODE_FAILED", "绑定码创建失败")
		return
	}
	writeData(w, map[string]any{"bindingCode": code, "expiresAt": expiresAt})
}

func (s *Server) writeSessionResponse(w http.ResponseWriter, r *http.Request, user mysql.User) {
	token, err := s.store.CreateSession(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUTH_FAILED", "会话创建失败")
		return
	}
	writeData(w, map[string]any{"token": token, "user": user})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token != "" {
		_ = s.store.Logout(r.Context(), token)
	}
	writeData(w, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeData(w, currentUser(r))
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	user, err := s.store.UpdateUser(r.Context(), currentUser(r).ID, req.DisplayName, req.Avatar)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", "用户信息保存失败")
		return
	}
	writeData(w, user)
}

func (s *Server) getProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.store.Profiles(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", "FIRE 档案读取失败")
		return
	}
	writeData(w, map[string]any{"profiles": profiles})
}

func (s *Server) syncProfiles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON")
		return
	}
	if err := s.store.SyncProfiles(r.Context(), currentUser(r).ID, req.Profiles); err != nil {
		log.Printf("sync profiles: %v", err)
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "FIRE 档案保存失败")
		return
	}
	writeData(w, map[string]any{"profiles": req.Profiles})
}

func (s *Server) emptyList(w http.ResponseWriter, r *http.Request) {
	writeData(w, []any{})
}

func (s *Server) echoCreated(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	writeJSON(w, http.StatusCreated, map[string]any{"data": payload})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "资源不存在")
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "请先登录")
			return
		}
		user, err := s.store.UserByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "登录已过期")
				return
			}
			writeError(w, http.StatusInternalServerError, "AUTH_FAILED", "认证失败")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(r *http.Request) mysql.User {
	user, _ := r.Context().Value(userKey).(mysql.User)
	return user
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func isLocalBootstrapRequest(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func writeData(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
