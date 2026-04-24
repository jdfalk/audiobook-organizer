// file: internal/database/ai_jobs_store.go
// version: 1.0.0
// guid: eb57745a-4c4f-4545-be8f-8d78b1e318f2

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AIJob is one tracked bulk LLM job submitted through the aijobs package.
type AIJob struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	BatchID        string    `json:"batch_id,omitempty"`
	CustomIDPrefix string    `json:"custom_id_prefix"`
	Status         string    `json:"status"` // pending|submitted|completed|completed_with_errors|failed|expired
	ItemCount      int       `json:"item_count"`
	SuccessCount   int       `json:"success_count"`
	ErrorCount     int       `json:"error_count"`
	RowErrors      string    `json:"row_errors,omitempty"` // JSON-encoded []AIJobRowError
	ErrorMsg       string    `json:"error_msg,omitempty"`
	SubmittedAt    time.Time `json:"submitted_at,omitempty"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AIJobRowError is one failed row within an otherwise successful batch.
type AIJobRowError struct {
	CustomID string `json:"custom_id"`
	Error    string `json:"error"`
}

const maxRowErrorsStored = 100

// CreateAIJob inserts the job row and its payload atomically.
func (s *SQLiteStore) CreateAIJob(job AIJob, payloadJSON []byte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO ai_jobs
		(id, type, batch_id, custom_id_prefix, status, item_count, success_count, error_count, row_errors, error_msg, submitted_at, completed_at, created_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, 0, 0, NULL, NULL, NULL, NULL, ?)`,
		job.ID, job.Type, job.BatchID, job.CustomIDPrefix, job.Status, job.ItemCount, job.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert ai_job: %w", err)
	}
	_, err = tx.Exec(`INSERT INTO ai_job_payloads (job_id, items_json) VALUES (?, ?)`, job.ID, payloadJSON)
	if err != nil {
		return fmt.Errorf("insert ai_job_payload: %w", err)
	}
	return tx.Commit()
}

// GetAIJob returns the job row by ID.
func (s *SQLiteStore) GetAIJob(id string) (AIJob, error) {
	return scanAIJob(s.db.QueryRow(aiJobSelectColumns+` FROM ai_jobs WHERE id = ?`, id))
}

// GetAIJobByBatchID returns the job row owning the given OpenAI batch ID.
func (s *SQLiteStore) GetAIJobByBatchID(batchID string) (AIJob, error) {
	return scanAIJob(s.db.QueryRow(aiJobSelectColumns+` FROM ai_jobs WHERE batch_id = ?`, batchID))
}

// GetAIJobPayload returns the stored items_json blob.
func (s *SQLiteStore) GetAIJobPayload(id string) ([]byte, error) {
	var b []byte
	err := s.db.QueryRow(`SELECT items_json FROM ai_job_payloads WHERE job_id = ?`, id).Scan(&b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// MarkAIJobSubmitted flips status to "submitted" and stamps batch_id + submitted_at.
func (s *SQLiteStore) MarkAIJobSubmitted(id, batchID string) error {
	_, err := s.db.Exec(`UPDATE ai_jobs SET status = 'submitted', batch_id = ?, submitted_at = ? WHERE id = ?`,
		batchID, time.Now(), id)
	return err
}

// MarkAIJobCompleted records terminal success (possibly with per-row errors).
func (s *SQLiteStore) MarkAIJobCompleted(id, status string, successCount, errorCount int, rowErrors []AIJobRowError) error {
	if len(rowErrors) > maxRowErrorsStored {
		rowErrors = rowErrors[:maxRowErrorsStored]
	}
	var errorsJSON []byte
	if len(rowErrors) > 0 {
		var err error
		errorsJSON, err = json.Marshal(rowErrors)
		if err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE ai_jobs
		SET status = ?, success_count = ?, error_count = ?, row_errors = ?, completed_at = ?
		WHERE id = ?`,
		status, successCount, errorCount, nullableBytes(errorsJSON), time.Now(), id)
	return err
}

// MarkAIJobFailed records job-level failure (submission error, expiry).
func (s *SQLiteStore) MarkAIJobFailed(id, errMsg string) error {
	_, err := s.db.Exec(`UPDATE ai_jobs SET status = 'failed', error_msg = ?, completed_at = ? WHERE id = ?`,
		errMsg, time.Now(), id)
	return err
}

// ListAIJobs returns jobs filtered by type/status (either may be empty to skip that filter).
func (s *SQLiteStore) ListAIJobs(typeFilter, statusFilter string, limit, offset int) ([]AIJob, error) {
	q := aiJobSelectColumns + ` FROM ai_jobs WHERE 1=1`
	args := []any{}
	if typeFilter != "" {
		q += ` AND type = ?`
		args = append(args, typeFilter)
	}
	if statusFilter != "" {
		q += ` AND status = ?`
		args = append(args, statusFilter)
	}
	q += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AIJob
	for rows.Next() {
		j, err := scanAIJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

const aiJobSelectColumns = `SELECT id, type, COALESCE(batch_id, ''), custom_id_prefix, status, item_count, success_count, error_count, COALESCE(row_errors, ''), COALESCE(error_msg, ''), submitted_at, completed_at, created_at`

func scanAIJob(r rowScanner) (AIJob, error) {
	return scanAIJobRow(r)
}

func scanAIJobRow(r rowScanner) (AIJob, error) {
	var j AIJob
	var submitted, completed sql.NullTime
	err := r.Scan(&j.ID, &j.Type, &j.BatchID, &j.CustomIDPrefix, &j.Status, &j.ItemCount,
		&j.SuccessCount, &j.ErrorCount, &j.RowErrors, &j.ErrorMsg, &submitted, &completed, &j.CreatedAt)
	if err != nil {
		return AIJob{}, err
	}
	if submitted.Valid {
		j.SubmittedAt = submitted.Time
	}
	if completed.Valid {
		j.CompletedAt = completed.Time
	}
	return j, nil
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
