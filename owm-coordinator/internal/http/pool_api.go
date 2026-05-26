package poolhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// dbQuerier is the minimal DB interface used internally by the handler.
// *pgxpool.Pool satisfies this interface in production; a hand-rolled mock
// satisfies it in tests without leaking any exported surface.
type dbQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// rowsQuerier is the interface for handlers that need to iterate over multiple rows.
type rowsQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type nodeStatusRow struct {
	NodeID string
	Tier   string
	Status string
}

type nodeStatusResponse struct {
	NodeID              string `json:"node_id"`
	Tier                string `json:"tier"`
	Status              string `json:"status"`
	CoordinatorVerified bool   `json:"coordinator_verified"`
}

// handleGetNodeStatus is the unexported implementation that accepts the narrow
// dbQuerier interface, keeping the logic testable without exposing the
// interface publicly.
func handleGetNodeStatus(db dbQuerier, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey := r.PathValue("pubkey")

		var row nodeStatusRow
		err := db.QueryRow(
			r.Context(),
			"SELECT node_id, tier, status FROM nodes WHERE public_key = $1",
			pubkey,
		).Scan(&row.NodeID, &row.Tier, &row.Status)

		w.Header().Set("Content-Type", "application/json")

		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "node not found"}) //nolint:errcheck
			return
		}
		if err != nil {
			log.Error("query node status", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(nodeStatusResponse{ //nolint:errcheck
			NodeID:              row.NodeID,
			Tier:                row.Tier,
			Status:              row.Status,
			CoordinatorVerified: true,
		})
	}
}

// HandleGetNodeStatus returns an http.HandlerFunc that looks up a node by its
// public key and reports its current status.
func HandleGetNodeStatus(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return handleGetNodeStatus(db, log)
}
