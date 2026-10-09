package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StartWorkers is called once after the application gateway is registered.
// Queued jobs live in the custom table; claiming is atomic across workers.
func (h *PelicanTestHandler) StartWorkers() {
	h.workerOnce.Do(func() {
		for i := 0; i < 2; i++ {
			go func() {
				for {
					if !h.workNextPelicanRun() {
						time.Sleep(time.Second)
					}
				}
			}()
		}
	})
}

func (h *PelicanTestHandler) workNextPelicanRun() bool {
	var id, userID, keyID, groupID int64
	var model, prompt, mode, requestID string
	claimCtx, claimCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := h.db.QueryRowContext(claimCtx, `UPDATE pelican_test_runs SET status='running', started_at=NOW()
	 WHERE id=(SELECT id FROM pelican_test_runs WHERE status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
	 RETURNING id, admin_user_id, api_key_id, group_id, model, prompt, output_mode, request_id`).Scan(
		&id, &userID, &keyID, &groupID, &model, &prompt, &mode, &requestID)
	claimCancel()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	h.cancelMu.Lock()
	h.active[id] = cancel
	h.cancelMu.Unlock()
	defer func() {
		cancel()
		h.cancelMu.Lock()
		delete(h.active, id)
		h.cancelMu.Unlock()
		if recover() != nil {
			h.failPelicanRun(id, "test worker failed")
		}
	}()
	// Cancellation may have committed between the claim and registering cancel.
	var status string
	if err := h.db.QueryRowContext(ctx, `SELECT status FROM pelican_test_runs WHERE id=$1`, id).Scan(&status); err != nil {
		h.failPelicanRun(id, "failed to read queued test")
		return true
	}
	if status != "running" {
		return true
	}
	key, err := h.ownKey(ctx, userID, keyID)
	if err != nil || key.GroupID == nil || *key.GroupID != groupID {
		h.failPelicanRun(id, "test key is unavailable or its group has changed")
		return true
	}
	h.executeConfiguredRun(ctx, id, key.Key, model, requestID, prompt, mode)
	return true
}

func (h *PelicanTestHandler) failPelicanRun(id int64, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = h.db.ExecContext(ctx, `UPDATE pelican_test_runs SET status='error', error_message=$2,
	 finished_at=NOW() WHERE id=$1 AND status='running'`, id, message)
}

func validPelicanPrompt(prompt, mode string) bool {
	return strings.TrimSpace(prompt) != "" && utf8.RuneCountInString(prompt) <= 16000 && (mode == "svg" || mode == "text")
}

func (h *PelicanTestHandler) createRuns(c *gin.Context, batch bool) {
	userID, ok := pelicanAdminID(c)
	if !ok {
		return
	}
	var body struct {
		KeyID      int64    `json:"key_id"`
		Model      string   `json:"model"`
		Models     []string `json:"models"`
		Prompt     *string  `json:"prompt"`
		OutputMode string   `json:"output_mode"`
	}
	if c.ShouldBindJSON(&body) != nil || body.KeyID <= 0 {
		response.BadRequest(c, "valid key_id and model selection required")
		return
	}
	if !batch {
		body.Models = []string{body.Model}
	}
	if len(body.Models) == 0 || len(body.Models) > 50 {
		response.BadRequest(c, "select between 1 and 50 models")
		return
	}
	prompt := pelicanPrompt
	if body.Prompt != nil {
		prompt = *body.Prompt
	}
	if body.OutputMode == "" {
		body.OutputMode = "svg"
	}
	if !validPelicanPrompt(prompt, body.OutputMode) {
		response.BadRequest(c, "prompt must contain 1–16000 characters; output_mode must be svg or text")
		return
	}
	ctx := c.Request.Context()
	key, err := h.ownKey(ctx, userID, body.KeyID)
	if err != nil {
		response.Error(c, http.StatusForbidden, err.Error())
		return
	}
	available, err := h.availableModels(ctx, key)
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	allowed := make(map[string]bool, len(available))
	for _, model := range available {
		allowed[model] = true
	}
	models := make([]string, 0, len(body.Models))
	seen := make(map[string]bool)
	for _, name := range body.Models {
		name = strings.TrimSpace(name)
		if len(name) > 200 || !allowed[name] || !pelicanTextModel(name) {
			response.BadRequest(c, "selected model is unavailable for this key")
			return
		}
		if !seen[name] {
			models = append(models, name)
			seen[name] = true
		}
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		response.InternalError(c, "failed to create test batch")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, "pelican-queue:"+strconv.FormatInt(userID, 10)); err != nil {
		response.InternalError(c, "failed to lock test queue")
		return
	}
	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pelican_test_runs WHERE admin_user_id=$1 AND status IN ('queued','running')`, userID).Scan(&pending); err != nil {
		response.InternalError(c, "failed to read test queue")
		return
	}
	if pending+len(models) > 100 {
		response.Error(c, http.StatusTooManyRequests, "at most 100 pending tests per administrator")
		return
	}
	batchID := uuid.NewString()
	ids := make([]int64, 0, len(models))
	firstRequest := ""
	for _, model := range models {
		requestID := uuid.NewString()
		if firstRequest == "" {
			firstRequest = requestID
		}
		var id int64
		err = tx.QueryRowContext(ctx, `INSERT INTO pelican_test_runs
		 (admin_user_id,api_key_id,group_id,model,prompt_version,request_id,status,prompt,output_mode,batch_id)
		 VALUES ($1,$2,$3,$4,'custom-v2',$5,'queued',$6,$7,$8) RETURNING id`, userID, key.ID, *key.GroupID, model, requestID, prompt, body.OutputMode, batchID).Scan(&id)
		if err != nil {
			response.InternalError(c, "failed to enqueue test")
			return
		}
		ids = append(ids, id)
	}
	if err = tx.Commit(); err != nil {
		response.InternalError(c, "failed to save test batch")
		return
	}
	response.Accepted(c, gin.H{"batch_id": batchID, "ids": ids, "id": ids[0], "request_id": firstRequest})
}

func (h *PelicanTestHandler) GetBatch(c *gin.Context) {
	if _, ok := pelicanAdminID(c); !ok {
		return
	}
	if _, err := uuid.Parse(c.Param("batch")); err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}
	h.markStale(c.Request.Context())
	rows, err := h.db.QueryContext(c.Request.Context(), pelicanRunSelect+` WHERE r.batch_id=$1 ORDER BY r.id LIMIT 50`, c.Param("batch"))
	if err != nil {
		response.InternalError(c, "failed to load test batch")
		return
	}
	defer rows.Close()
	runs := make([]*pelicanRun, 0)
	for rows.Next() {
		r, err := scanPelicanRun(rows)
		if err != nil {
			response.InternalError(c, "failed to read test batch")
			return
		}
		runs = append(runs, r)
	}
	if rows.Err() != nil {
		response.InternalError(c, "failed to read test batch")
		return
	}
	response.Success(c, gin.H{"items": runs})
}

func (h *PelicanTestHandler) CancelRun(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid run id")
		return
	}
	h.cancelPelicanRuns(c, id, "")
}

func (h *PelicanTestHandler) CancelBatch(c *gin.Context) {
	if _, err := uuid.Parse(c.Param("batch")); err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}
	h.cancelPelicanRuns(c, 0, c.Param("batch"))
}

func (h *PelicanTestHandler) cancelPelicanRuns(c *gin.Context, id int64, batchID string) {
	userID, ok := pelicanAdminID(c)
	if !ok {
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `UPDATE pelican_test_runs SET status='cancelled',
	 error_message='cancelled by administrator',finished_at=NOW(),
	 duration_ms=CASE WHEN started_at IS NULL THEN 0 ELSE (EXTRACT(EPOCH FROM NOW()-started_at)*1000)::bigint END
	 WHERE admin_user_id=$1 AND (($2::bigint>0 AND id=$2) OR ($3::text<>'' AND batch_id=$3))
	 AND status IN ('queued','running') RETURNING id`, userID, id, batchID)
	if err != nil {
		response.InternalError(c, "failed to cancel tests")
		return
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var runID int64
		if err := rows.Scan(&runID); err != nil {
			response.InternalError(c, "failed to read cancelled tests")
			return
		}
		ids = append(ids, runID)
	}
	if rows.Err() != nil {
		response.InternalError(c, "failed to cancel tests")
		return
	}
	h.cancelMu.Lock()
	for _, runID := range ids {
		if cancel := h.active[runID]; cancel != nil {
			cancel()
		}
	}
	h.cancelMu.Unlock()
	response.Success(c, gin.H{"cancelled": len(ids)})
}

type pelicanTemplate struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prompt     string `json:"prompt"`
	OutputMode string `json:"output_mode"`
	IsDefault  bool   `json:"is_default"`
}

func (h *PelicanTestHandler) ListTemplates(c *gin.Context) {
	userID, ok := pelicanAdminID(c)
	if !ok {
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT id,name,prompt,output_mode,is_default FROM pelican_prompt_templates WHERE admin_user_id=$1 ORDER BY is_default DESC,id DESC`, userID)
	if err != nil {
		response.InternalError(c, "failed to load prompt templates")
		return
	}
	defer rows.Close()
	items := make([]pelicanTemplate, 0)
	for rows.Next() {
		var item pelicanTemplate
		if err := rows.Scan(&item.ID, &item.Name, &item.Prompt, &item.OutputMode, &item.IsDefault); err != nil {
			response.InternalError(c, "failed to read prompt templates")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		response.InternalError(c, "failed to read prompt templates")
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *PelicanTestHandler) SaveTemplate(c *gin.Context) {
	userID, ok := pelicanAdminID(c)
	if !ok {
		return
	}
	id := int64(0)
	if c.Param("id") != "" {
		var err error
		id, err = strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "invalid template id")
			return
		}
	}
	var body pelicanTemplate
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "invalid template")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || utf8.RuneCountInString(body.Name) > 100 || !validPelicanPrompt(body.Prompt, body.OutputMode) {
		response.BadRequest(c, "template requires a name (1–100 characters), prompt (1–16000 characters), and svg or text output mode")
		return
	}
	ctx := c.Request.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		response.InternalError(c, "failed to save template")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, "pelican-templates:"+strconv.FormatInt(userID, 10)); err != nil {
		response.InternalError(c, "failed to lock templates")
		return
	}
	if id == 0 {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pelican_prompt_templates WHERE admin_user_id=$1`, userID).Scan(&count); err != nil {
			response.InternalError(c, "failed to read templates")
			return
		}
		if count >= 50 {
			response.BadRequest(c, "at most 50 prompt templates per administrator")
			return
		}
	}
	if body.IsDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE pelican_prompt_templates SET is_default=FALSE WHERE admin_user_id=$1 AND is_default`, userID); err != nil {
			response.InternalError(c, "failed to update default template")
			return
		}
	}
	if id == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO pelican_prompt_templates (admin_user_id,name,prompt,output_mode,is_default) VALUES ($1,$2,$3,$4,$5) RETURNING id`, userID, body.Name, body.Prompt, body.OutputMode, body.IsDefault).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE pelican_prompt_templates SET name=$3,prompt=$4,output_mode=$5,is_default=$6,updated_at=NOW() WHERE id=$1 AND admin_user_id=$2 RETURNING id`, id, userID, body.Name, body.Prompt, body.OutputMode, body.IsDefault).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(c, "template not found")
		return
	}
	if err != nil {
		response.InternalError(c, "failed to save template")
		return
	}
	if err = tx.Commit(); err != nil {
		response.InternalError(c, "failed to save template")
		return
	}
	body.ID = id
	response.Success(c, body)
}

func (h *PelicanTestHandler) DeleteTemplate(c *gin.Context) {
	userID, ok := pelicanAdminID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid template id")
		return
	}
	result, err := h.db.ExecContext(c.Request.Context(), `DELETE FROM pelican_prompt_templates WHERE id=$1 AND admin_user_id=$2`, id, userID)
	if err != nil {
		response.InternalError(c, "failed to delete template")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		response.NotFound(c, "template not found")
		return
	}
	response.Success(c, gin.H{"deleted": id})
}
