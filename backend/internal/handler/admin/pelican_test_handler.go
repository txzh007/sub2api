package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	pelicanPrompt      = "Generate an SVG of a pelican riding a bicycle"
	pelicanMaxResponse = 1 << 20
	pelicanMaxSVG      = 512 << 10
	pelicanTestTimeout = 3 * time.Minute
	pelicanPromptVer   = "simon-2024-v1"
)

var pelicanSVGPattern = regexp.MustCompile(`(?is)<svg(?:\s|>).*?</svg\s*>`)
var pelicanLocalURLPattern = regexp.MustCompile(`(?i)url\(\s*['"]?#[a-z_][a-z0-9_.:-]*['"]?\s*\)`)

// PelicanTestHandler keeps test-only state outside of the normal account test path.
// The injected gateway is the already-registered HTTP router, so authentication,
// mapping, scheduling and billing are identical to a user's gateway request.
type PelicanTestHandler struct {
	db         *sql.DB
	keys       service.APIKeyRepository
	gateway    http.Handler
	slots      chan struct{}
	timeout    time.Duration
	workerOnce sync.Once
	cancelMu   sync.Mutex
	active     map[int64]context.CancelFunc
}

func NewPelicanTestHandler(db *sql.DB, keys service.APIKeyRepository) *PelicanTestHandler {
	return &PelicanTestHandler{db: db, keys: keys, slots: make(chan struct{}, 2), timeout: pelicanTestTimeout, active: make(map[int64]context.CancelFunc)}
}

func (h *PelicanTestHandler) SetGateway(gateway http.Handler) { h.gateway = gateway }

type pelicanKeyOption struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
}

type pelicanRun struct {
	ID             int64      `json:"id"`
	AdminUserID    int64      `json:"admin_user_id"`
	APIKeyID       int64      `json:"api_key_id"`
	GroupID        int64      `json:"group_id"`
	GroupName      string     `json:"group_name"`
	Model          string     `json:"model"`
	Prompt         string     `json:"prompt"`
	OutputMode     string     `json:"output_mode"`
	BatchID        string     `json:"batch_id"`
	PromptVer      string     `json:"prompt_version"`
	RequestID      string     `json:"request_id"`
	UsageRequestID *string    `json:"usage_request_id"`
	Status         string     `json:"status"`
	ResponseText   string     `json:"response_text,omitempty"`
	SVG            string     `json:"svg,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	DurationMS     int64      `json:"duration_ms"`
	InputTokens    *int64     `json:"input_tokens"`
	OutputTokens   *int64     `json:"output_tokens"`
	TotalCost      *float64   `json:"total_cost"`
	ActualCost     *float64   `json:"actual_cost"`
	CreatedAt      time.Time  `json:"created_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

func pelicanAdminID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "administrator required")
		return 0, false
	}
	return subject.UserID, true
}

func (h *PelicanTestHandler) ownKey(ctx context.Context, userID, keyID int64) (*service.APIKey, error) {
	key, err := h.keys.GetByID(ctx, keyID)
	if err != nil || key == nil || key.UserID != userID || !key.IsActive() || key.GroupID == nil || key.Group == nil || !key.Group.IsActive() {
		return nil, errors.New("active group-bound test key not found for this administrator")
	}
	if len(key.IPWhitelist) > 0 || len(key.IPBlacklist) > 0 {
		return nil, errors.New("test key must not have IP restrictions")
	}
	return key, nil
}

// Options returns only the caller's own key metadata, never the secret.
func (h *PelicanTestHandler) Options(c *gin.Context) {
	userID, ok := pelicanAdminID(c)
	if !ok {
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT k.id, k.name, g.id, g.name
		FROM api_keys k JOIN groups g ON g.id = k.group_id
		WHERE k.user_id = $1 AND k.status = 'active' AND k.deleted_at IS NULL
		  AND g.status = 'active' AND g.deleted_at IS NULL
		ORDER BY g.name, k.name, k.id`, userID)
	if err != nil {
		response.InternalError(c, "failed to load test keys")
		return
	}
	defer rows.Close()
	options := make([]pelicanKeyOption, 0)
	for rows.Next() {
		var item pelicanKeyOption
		if err := rows.Scan(&item.ID, &item.Name, &item.GroupID, &item.GroupName); err != nil {
			response.InternalError(c, "failed to load test keys")
			return
		}
		options = append(options, item)
	}
	if rows.Err() != nil {
		response.InternalError(c, "failed to load test keys")
		return
	}
	response.Success(c, gin.H{"keys": options, "prompt": pelicanPrompt})
}

func pelicanTextModel(name string) bool {
	lower := strings.ToLower(name)
	for _, fragment := range []string{"image", "video", "seedance", "sora", "embedding", "audio", "realtime", "tts", "whisper"} {
		if strings.Contains(lower, fragment) {
			return false
		}
	}
	return name != ""
}

func (h *PelicanTestHandler) availableModels(ctx context.Context, key *service.APIKey) ([]string, error) {
	result, err := h.callGateway(ctx, http.MethodGet, "/v1/models", key.Key, "", "")
	if err != nil {
		return nil, err
	}
	if result.status != http.StatusOK {
		return nil, fmt.Errorf("model list returned HTTP %d", result.status)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result.body, &body); err != nil {
		return nil, errors.New("invalid model list response")
	}
	models := make([]string, 0, len(body.Data))
	for _, model := range body.Data {
		if pelicanTextModel(model.ID) {
			models = append(models, model.ID)
		}
	}
	return models, nil
}

func (h *PelicanTestHandler) Models(c *gin.Context) {
	userID, ok := pelicanAdminID(c)
	if !ok {
		return
	}
	keyID, err := strconv.ParseInt(c.Query("key_id"), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "valid key_id required")
		return
	}
	key, err := h.ownKey(c.Request.Context(), userID, keyID)
	if err != nil {
		response.Error(c, http.StatusForbidden, err.Error())
		return
	}
	models, err := h.availableModels(c.Request.Context(), key)
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Success(c, gin.H{"models": models})
}

func (h *PelicanTestHandler) CreateRun(c *gin.Context)   { h.createRuns(c, false) }
func (h *PelicanTestHandler) CreateBatch(c *gin.Context) { h.createRuns(c, true) }

type gatewayResult struct {
	status   int
	body     []byte
	tooLarge bool
}

type pelicanResponseWriter struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	tooLarge bool
}

func (w *pelicanResponseWriter) Header() http.Header { return w.header }
func (w *pelicanResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *pelicanResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(p) > pelicanMaxResponse {
		w.tooLarge = true
		return 0, errors.New("gateway response exceeded 1 MiB")
	}
	return w.body.Write(p)
}
func (w *pelicanResponseWriter) Flush() {}

func (h *PelicanTestHandler) callGateway(ctx context.Context, method, path, secret, requestID, body string) (*gatewayResult, error) {
	if h.gateway == nil {
		return nil, errors.New("gateway is not ready")
	}
	if requestID != "" {
		ctx = context.WithValue(ctx, ctxkey.ClientRequestID, requestID)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-Client-Request-ID", requestID)
	req.Header.Set("User-Agent", "TToken-Pelican-Test/1")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = "127.0.0.1:0"
	w := &pelicanResponseWriter{header: make(http.Header)}
	h.gateway.ServeHTTP(w, req)
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return &gatewayResult{status: w.status, body: w.body.Bytes(), tooLarge: w.tooLarge}, nil
}

func (h *PelicanTestHandler) executeRun(id int64, secret, model, requestID string) {
	defer func() { <-h.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	h.executeConfiguredRun(ctx, id, secret, model, requestID, pelicanPrompt, "svg")
}

func (h *PelicanTestHandler) executeConfiguredRun(ctx context.Context, id int64, secret, model, requestID, prompt, outputMode string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = h.db.ExecContext(ctx, `UPDATE pelican_test_runs SET status='error', error_message='test worker failed', finished_at=NOW() WHERE id=$1 AND status='running'`, id)
		}
	}()
	started := time.Now()
	status, raw, svg, message := "error", "", "", ""
	body, _ := json.Marshal(gin.H{
		"model": model, "stream": false, "max_tokens": 16384,
		"messages": []gin.H{{"role": "user", "content": prompt}},
	})
	result, err := h.callGateway(ctx, http.MethodPost, "/v1/chat/completions", secret, requestID, string(body))
	switch {
	case err != nil:
		message = err.Error()
	case ctx.Err() != nil:
		message = "test timed out"
	case result.tooLarge:
		message = "gateway response exceeded 1 MiB"
	case result.status != http.StatusOK:
		message = fmt.Sprintf("gateway returned HTTP %d", result.status)
		raw = truncatePelican(string(result.body), pelicanMaxSVG)
		var failure struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(result.body, &failure) == nil && failure.Error.Message != "" {
			message += ": " + truncatePelican(failure.Error.Message, 500)
		}
	default:
		var completion struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(result.body, &completion) != nil || len(completion.Choices) == 0 {
			message = "gateway response did not contain chat content"
		} else {
			content := completion.Choices[0].Message.Content
			raw = truncatePelican(content, pelicanMaxSVG)
			if outputMode == "text" {
				if strings.TrimSpace(content) == "" {
					message = "model returned empty content"
				} else {
					status = "success"
				}
			} else {
				svg, err = extractSafePelicanSVG(content)
				if err != nil {
					status = "invalid_svg"
					message = err.Error()
				} else {
					status = "success"
				}
			}
		}
	}
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()
	_, _ = h.db.ExecContext(writeCtx, `
		UPDATE pelican_test_runs SET status=$2, response_text=$3, svg=$4, error_message=$5,
		 duration_ms=$6, finished_at=NOW() WHERE id=$1 AND status='running'`, id, status, raw, svg,
		truncatePelican(message, 1000), time.Since(started).Milliseconds())
}

func truncatePelican(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return strings.ToValidUTF8(value[:max], "")
}

func extractSafePelicanSVG(raw string) (string, error) {
	svg := pelicanSVGPattern.FindString(raw)
	if svg == "" {
		return "", errors.New("model did not return an SVG")
	}
	if len(svg) > pelicanMaxSVG {
		return "", errors.New("SVG exceeds 512 KiB")
	}
	dec := xml.NewDecoder(strings.NewReader(svg))
	depth := 0
	inStyle := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("SVG is not valid XML")
		}
		switch t := tok.(type) {
		case xml.Directive, xml.ProcInst:
			return "", errors.New("SVG contains an unsafe directive")
		case xml.StartElement:
			if depth == 0 && strings.ToLower(t.Name.Local) != "svg" {
				return "", errors.New("SVG root element is invalid")
			}
			depth++
			name := strings.ToLower(t.Name.Local)
			if strings.HasPrefix(name, "animate") {
				return "", errors.New("SVG contains unsafe elements")
			}
			switch name {
			case "script", "foreignobject", "image", "iframe", "object", "embed", "animate", "set", "mpath":
				return "", errors.New("SVG contains unsafe elements")
			}
			if name == "style" {
				inStyle = true
			}
			for _, attr := range t.Attr {
				name := strings.ToLower(attr.Name.Local)
				value := strings.ToLower(strings.TrimSpace(attr.Value))
				if strings.HasPrefix(name, "on") || name == "src" || unsafePelicanCSS(value) ||
					(name == "href" && !strings.HasPrefix(value, "#")) {
					return "", errors.New("SVG contains unsafe attributes")
				}
			}
		case xml.CharData:
			if inStyle && unsafePelicanCSS(strings.ToLower(string(t))) {
				return "", errors.New("SVG contains unsafe styles")
			}
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "style") {
				inStyle = false
			}
			depth--
		}
	}
	if depth != 0 {
		return "", errors.New("SVG is incomplete")
	}
	return svg, nil
}

func unsafePelicanCSS(value string) bool {
	withoutLocalReferences := pelicanLocalURLPattern.ReplaceAllString(value, "")
	return strings.Contains(withoutLocalReferences, "url(") || strings.Contains(value, "@import") ||
		strings.Contains(value, "expression(") || strings.Contains(value, "javascript:")
}

const pelicanRunSelect = `SELECT r.id, r.admin_user_id, r.api_key_id, r.group_id, COALESCE(g.name,''),
 r.model, r.prompt, r.output_mode, r.batch_id, r.prompt_version, r.request_id, r.status, r.response_text, r.svg, r.error_message,
 r.duration_ms, r.created_at, r.finished_at, u.request_id, u.input_tokens, u.output_tokens, u.total_cost, u.actual_cost
 FROM pelican_test_runs r LEFT JOIN groups g ON g.id=r.group_id
 LEFT JOIN LATERAL (SELECT request_id, input_tokens, output_tokens, total_cost, actual_cost FROM usage_logs
   WHERE request_id IN (r.request_id, 'client:' || r.request_id) AND api_key_id=r.api_key_id ORDER BY id DESC LIMIT 1) u ON true`

var pelicanRunListSelect = strings.Replace(pelicanRunSelect, "r.response_text, r.svg", "''::text, ''::text", 1)

func scanPelicanRun(row interface{ Scan(...any) error }) (*pelicanRun, error) {
	r := &pelicanRun{}
	var in, out sql.NullInt64
	var total, actual sql.NullFloat64
	var usageRequestID sql.NullString
	var finished sql.NullTime
	err := row.Scan(&r.ID, &r.AdminUserID, &r.APIKeyID, &r.GroupID, &r.GroupName,
		&r.Model, &r.Prompt, &r.OutputMode, &r.BatchID, &r.PromptVer, &r.RequestID, &r.Status, &r.ResponseText, &r.SVG,
		&r.ErrorMessage, &r.DurationMS, &r.CreatedAt, &finished, &usageRequestID, &in, &out, &total, &actual)
	if err != nil {
		return nil, err
	}
	if finished.Valid {
		r.FinishedAt = &finished.Time
	}
	if usageRequestID.Valid {
		r.UsageRequestID = &usageRequestID.String
	}
	if in.Valid {
		r.InputTokens = &in.Int64
	}
	if out.Valid {
		r.OutputTokens = &out.Int64
	}
	if total.Valid {
		r.TotalCost = &total.Float64
	}
	if actual.Valid {
		r.ActualCost = &actual.Float64
	}
	return r, nil
}

func (h *PelicanTestHandler) markStale(ctx context.Context) {
	_, _ = h.db.ExecContext(ctx, `UPDATE pelican_test_runs SET status='interrupted',
	 error_message='test interrupted by a server restart or timeout', finished_at=NOW()
	 WHERE status='running' AND COALESCE(started_at, created_at) < NOW() - INTERVAL '4 minutes'`)
}

func (h *PelicanTestHandler) ListRuns(c *gin.Context) {
	if _, ok := pelicanAdminID(c); !ok {
		return
	}
	h.markStale(c.Request.Context())
	groupID, _ := strconv.ParseInt(c.Query("group_id"), 10, 64)
	beforeID, err := strconv.ParseInt(c.DefaultQuery("before_id", "0"), 10, 64)
	if err != nil || beforeID < 0 {
		response.BadRequest(c, "invalid before_id")
		return
	}
	model := strings.TrimSpace(c.Query("model"))
	if len(model) > 200 {
		response.BadRequest(c, "model filter is too long")
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), pelicanRunListSelect+`
	 WHERE ($1::bigint=0 OR r.group_id=$1) AND ($2::text='' OR r.model=$2)
	   AND ($3::bigint=0 OR r.id<$3)
	 ORDER BY r.id DESC LIMIT 51`, groupID, model, beforeID)
	if err != nil {
		response.InternalError(c, "failed to load test history")
		return
	}
	defer rows.Close()
	runs := make([]*pelicanRun, 0)
	hasMore := false
	for rows.Next() {
		if len(runs) == 50 {
			hasMore = true
			break
		}
		r, err := scanPelicanRun(rows)
		if err != nil {
			response.InternalError(c, "failed to load test history")
			return
		}
		r.ResponseText, r.SVG = "", ""
		runs = append(runs, r)
	}
	if rows.Err() != nil {
		response.InternalError(c, "failed to load test history")
		return
	}
	nextBeforeID := int64(0)
	if hasMore {
		nextBeforeID = runs[len(runs)-1].ID
	}
	response.Success(c, gin.H{"items": runs, "next_before_id": nextBeforeID})
}

func (h *PelicanTestHandler) GetRun(c *gin.Context) {
	if _, ok := pelicanAdminID(c); !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid run id")
		return
	}
	h.markStale(c.Request.Context())
	r, err := scanPelicanRun(h.db.QueryRowContext(c.Request.Context(), pelicanRunSelect+` WHERE r.id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(c, "test run not found")
		return
	}
	if err != nil {
		response.InternalError(c, "failed to load test run")
		return
	}
	response.Success(c, r)
}
