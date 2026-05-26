package poolhttp

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type slashingEventRow struct {
	EventID           string `json:"event_id"`
	NodeID            string `json:"node_id"`
	ChannelID         string `json:"channel_id"`
	Reason            string `json:"reason"`
	Evidence          string `json:"evidence"`
	SlashedAt         string `json:"slashed_at"`
	CooldownExpiresAt string `json:"cooldown_expires_at"`
}

type slashingLogResponse struct {
	Events []slashingEventRow `json:"events"`
	Total  int                `json:"total"`
}

type slashingLogDB interface {
	dbQuerier
	rowsQuerier
}

func handleGetSlashingLog(db slashingLogDB, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.URL.Query().Get("node_id")
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

		var total int
		countSQL := `SELECT COUNT(*) FROM slashing_events`
		var countArgs []any
		if nodeID != "" {
			countSQL += ` WHERE node_id = $1`
			countArgs = []any{nodeID}
		}
		err := db.QueryRow(r.Context(), countSQL, countArgs...).Scan(&total)
		if err != nil {
			log.Error("count slashing events", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}

		dataSQL := `SELECT event_id, node_id, channel_id, reason,
			evidence_hash AS evidence, executed_at AS slashed_at, cooldown_expires_at
			FROM slashing_events`
		var dataArgs []any
		if nodeID != "" {
			dataSQL += ` WHERE node_id = $1 ORDER BY executed_at DESC LIMIT $2 OFFSET $3`
			dataArgs = []any{nodeID, limit, offset}
		} else {
			dataSQL += ` ORDER BY executed_at DESC LIMIT $1 OFFSET $2`
			dataArgs = []any{limit, offset}
		}

		rows, err := db.Query(r.Context(), dataSQL, dataArgs...)
		if err != nil {
			log.Error("query slashing events", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}
		defer rows.Close()

		events := []slashingEventRow{}
		for rows.Next() {
			var ev slashingEventRow
			err := rows.Scan(&ev.EventID, &ev.NodeID, &ev.ChannelID, &ev.Reason, &ev.Evidence, &ev.SlashedAt, &ev.CooldownExpiresAt)
			if err != nil {
				log.Error("scan slashing row", zap.Error(err))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
				return
			}
			events = append(events, ev)
		}
		if err := rows.Err(); err != nil {
			log.Error("slashing rows error", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(slashingLogResponse{Events: events, Total: total}) //nolint:errcheck
	}
}

// HandleGetSlashingLog returns an http.HandlerFunc that returns the slashing log.
func HandleGetSlashingLog(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return handleGetSlashingLog(db, log)
}