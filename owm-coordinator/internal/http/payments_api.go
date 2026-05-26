package poolhttp

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type paymentRow struct {
	TaskID      string  `json:"task_id"`
	NodeID      string  `json:"node_id"`
	NodeLNURI   string  `json:"node_ln_uri"`
	TaskType    string  `json:"task_type"`
	Status      string  `json:"status"`
	RewardSats  int     `json:"reward_sats"`
	SubmittedAt string  `json:"submitted_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

type paymentsHistoryResponse struct {
	Payments []paymentRow `json:"payments"`
	Total    int          `json:"total"`
}

type paymentsDB interface {
	dbQuerier
	rowsQuerier
}

func handleGetPaymentsHistory(db paymentsDB, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.URL.Query().Get("node_id")
		status := r.URL.Query().Get("status")
		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		if nodeID != "" {
			if _, err := uuid.Parse(nodeID); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid node_id"}) //nolint:errcheck
				return
			}
		}

		if status != "" {
			if status != "pending" && status != "assigned" && status != "completed" && status != "failed" && status != "timeout" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid status"}) //nolint:errcheck
				return
			}
		}

		limit := 50
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}
		if limit > 200 {
			limit = 200
		}

		offset := 0
		if offsetStr != "" {
			if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
				offset = o
			}
		}

		// Build count query
		countSQL := `SELECT COUNT(*) FROM tasks WHERE task_type = 'inference'`
		countArgs := []any{}
		argIdx := 1
		if nodeID != "" {
			countSQL += ` AND assigned_node = $` + strconv.Itoa(argIdx)
			countArgs = append(countArgs, nodeID)
			argIdx++
		}
		if status != "" {
			countSQL += ` AND status = $` + strconv.Itoa(argIdx)
			countArgs = append(countArgs, status)
		}

		var total int
		err := db.QueryRow(r.Context(), countSQL, countArgs...).Scan(&total)
		if err != nil {
			log.Error("count payments", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}

		// Build data query
		dataSQL := `SELECT task_id, assigned_node, node_ln_uri, task_type, status,
			reward_sats, submitted_at, completed_at
			FROM tasks WHERE task_type = 'inference'`
		dataArgs := []any{}
		argIdx = 1
		if nodeID != "" {
			dataSQL += ` AND assigned_node = $` + strconv.Itoa(argIdx)
			dataArgs = append(dataArgs, nodeID)
			argIdx++
		}
		if status != "" {
			dataSQL += ` AND status = $` + strconv.Itoa(argIdx)
			dataArgs = append(dataArgs, status)
			argIdx++
		}
		dataSQL += ` ORDER BY submitted_at DESC LIMIT $` + strconv.Itoa(argIdx)
		dataArgs = append(dataArgs, limit)
		argIdx++
		dataSQL += ` OFFSET $` + strconv.Itoa(argIdx)
		dataArgs = append(dataArgs, offset)

		rows, err := db.Query(r.Context(), dataSQL, dataArgs...)
		if err != nil {
			log.Error("query payments", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}
		defer rows.Close()

		payments := []paymentRow{}
		for rows.Next() {
			var p paymentRow
			var completedAt *string
			err := rows.Scan(&p.TaskID, &p.NodeID, &p.NodeLNURI, &p.TaskType, &p.Status, &p.RewardSats, &p.SubmittedAt, &completedAt)
			if err != nil {
				log.Error("scan payment row", zap.Error(err))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
				return
			}
			if completedAt != nil {
				p.CompletedAt = completedAt
			}
			payments = append(payments, p)
		}
		if err := rows.Err(); err != nil {
			log.Error("payment rows error", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(paymentsHistoryResponse{Payments: payments, Total: total}) //nolint:errcheck
	}
}

// HandleGetPaymentsHistory returns an http.HandlerFunc that returns payment history.
func HandleGetPaymentsHistory(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return handleGetPaymentsHistory(db, log)
}