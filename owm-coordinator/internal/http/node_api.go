package poolhttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// ── GET /v1/nodes/{node_id}/status ───────────────────────────────────────────

type nodeStatusDetailResponse struct {
	NodeID              string  `json:"node_id"`
	Tier                string  `json:"tier"`
	Status              string  `json:"status"`
	RegisteredAt        string  `json:"registered_at"`
	LastHeartbeat       *string `json:"last_heartbeat,omitempty"`
	Reliability         float64 `json:"reliability"`
	TotalTasksCompleted int     `json:"total_tasks_completed"`
	TotalSatsEarned     int64   `json:"total_sats_earned"`
}

// HandleGetNodeStatusV1 returns the current status of a specific node.
func HandleGetNodeStatusV1(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeIDStr := r.PathValue("node_id")
		if _, err := uuid.Parse(nodeIDStr); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid node_id"}) //nolint:errcheck
			return
		}

		var resp nodeStatusDetailResponse
		resp.NodeID = nodeIDStr
		var registeredAt time.Time
		var lastHeartbeat *time.Time

		err := db.QueryRow(r.Context(), `
			SELECT tier, status, registered_at, last_heartbeat,
			       reliability, total_tasks, total_sats
			FROM nodes WHERE node_id = $1`,
			nodeIDStr,
		).Scan(
			&resp.Tier, &resp.Status, &registeredAt, &lastHeartbeat,
			&resp.Reliability, &resp.TotalTasksCompleted, &resp.TotalSatsEarned,
		)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "node not found"}) //nolint:errcheck
			return
		}
		resp.RegisteredAt = registeredAt.UTC().Format(time.RFC3339)
		if lastHeartbeat != nil {
			s := lastHeartbeat.UTC().Format(time.RFC3339)
			resp.LastHeartbeat = &s
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}

// ── GET /v1/nodes/{node_id}/earnings ─────────────────────────────────────────

type earningsTaskRow struct {
	TaskID      string  `json:"task_id"`
	TaskType    string  `json:"task_type"`
	RewardSats  int     `json:"reward_sats"`
	Status      string  `json:"status"`
	SubmittedAt string  `json:"submitted_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

type earningsResponse struct {
	NodeID         string            `json:"node_id"`
	TotalSatsEarned int64            `json:"total_sats_earned"`
	Tasks          []earningsTaskRow `json:"tasks"`
	Total          int               `json:"total"`
}

// HandleGetNodeEarnings returns the earnings history for a specific node.
func HandleGetNodeEarnings(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeIDStr := r.PathValue("node_id")
		if _, err := uuid.Parse(nodeIDStr); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid node_id"}) //nolint:errcheck
			return
		}

		limit := 50
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
			limit = l
		}
		if limit > 200 {
			limit = 200
		}
		offset := 0
		if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
			offset = o
		}

		// Fetch total sats and task count from nodes table.
		var totalSats int64
		var totalTasks int
		if err := db.QueryRow(r.Context(),
			`SELECT total_sats, total_tasks FROM nodes WHERE node_id = $1`, nodeIDStr,
		).Scan(&totalSats, &totalTasks); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "node not found"}) //nolint:errcheck
			return
		}

		rows, err := db.Query(r.Context(), `
			SELECT task_id, task_type, reward_sats, status, submitted_at, completed_at
			FROM tasks
			WHERE assigned_node = $1
			ORDER BY submitted_at DESC
			LIMIT $2 OFFSET $3`,
			nodeIDStr, limit, offset,
		)
		if err != nil {
			log.Error("earnings query", zap.String("node_id", nodeIDStr), zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}
		defer rows.Close()

		tasks := []earningsTaskRow{}
		for rows.Next() {
			var t earningsTaskRow
			var submittedAt time.Time
			var completedAt *time.Time
			if err := rows.Scan(&t.TaskID, &t.TaskType, &t.RewardSats, &t.Status,
				&submittedAt, &completedAt); err != nil {
				log.Error("earnings scan", zap.Error(err))
				continue
			}
			t.SubmittedAt = submittedAt.UTC().Format(time.RFC3339)
			if completedAt != nil {
				s := completedAt.UTC().Format(time.RFC3339)
				t.CompletedAt = &s
			}
			tasks = append(tasks, t)
		}
		if err := rows.Err(); err != nil {
			log.Error("earnings rows error", zap.Error(err))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(earningsResponse{ //nolint:errcheck
			NodeID:          nodeIDStr,
			TotalSatsEarned: totalSats,
			Tasks:           tasks,
			Total:           totalTasks,
		})
	}
}
