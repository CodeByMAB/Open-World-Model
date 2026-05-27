package poolhttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/stake"
)

// ── GET /v1/network/stats ────────────────────────────────────────────────────

type networkStatsResponse struct {
	ActiveNodes        int            `json:"active_nodes"`
	ActiveNodesByTier  map[string]int `json:"active_nodes_by_tier"`
	TotalNodes         int            `json:"total_nodes"`
	TasksCompleted     int64          `json:"tasks_completed_total"`
	TotalSatsPaid      int64          `json:"total_sats_paid"`
	ActiveFLRounds     int            `json:"active_fl_rounds"`
	NetworkReliability float64        `json:"network_reliability_avg"`
}

// HandleGetNetworkStats returns aggregate network-wide statistics.
func HandleGetNetworkStats(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var resp networkStatsResponse
		resp.ActiveNodesByTier = map[string]int{"t1": 0, "t2": 0, "t3": 0}

		var t1, t2, t3 int
		err := db.QueryRow(r.Context(), `
			SELECT
			  COUNT(*)                                             AS total_nodes,
			  COUNT(*) FILTER (WHERE status = 'active')           AS active_nodes,
			  COUNT(*) FILTER (WHERE status='active' AND tier='t1') AS t1,
			  COUNT(*) FILTER (WHERE status='active' AND tier='t2') AS t2,
			  COUNT(*) FILTER (WHERE status='active' AND tier='t3') AS t3,
			  COALESCE(SUM(total_tasks), 0)                       AS tasks_completed,
			  COALESCE(SUM(total_sats), 0)                        AS total_sats,
			  COALESCE(AVG(reliability) FILTER (WHERE status='active'), 0) AS avg_reliability
			FROM nodes`,
		).Scan(
			&resp.TotalNodes,
			&resp.ActiveNodes,
			&t1, &t2, &t3,
			&resp.TasksCompleted,
			&resp.TotalSatsPaid,
			&resp.NetworkReliability,
		)
		resp.ActiveNodesByTier["t1"] = t1
		resp.ActiveNodesByTier["t2"] = t2
		resp.ActiveNodesByTier["t3"] = t3
		if err != nil {
			log.Error("network stats query", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}

		if err := db.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM fl_rounds WHERE status IN ('open','aggregating')`,
		).Scan(&resp.ActiveFLRounds); err != nil {
			log.Warn("fl_rounds count", zap.Error(err))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}

// ── GET /v1/network/nodes ────────────────────────────────────────────────────

type networkNodeRow struct {
	NodeID       string  `json:"node_id"`
	Tier         string  `json:"tier"`
	Status       string  `json:"status"`
	RegisteredAt string  `json:"registered_at"`
	Reliability  float64 `json:"reliability"`
	TotalTasks   int     `json:"total_tasks"`
}

type networkNodesResponse struct {
	Nodes []networkNodeRow `json:"nodes"`
	Total int              `json:"total"`
}

// HandleGetNetworkNodes returns a paginated, anonymized list of active nodes.
func HandleGetNetworkNodes(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		var total int
		if err := db.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM nodes WHERE status = 'active'`,
		).Scan(&total); err != nil {
			log.Error("network nodes count", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}

		rows, err := db.Query(r.Context(), `
			SELECT node_id, tier, status, registered_at, reliability, total_tasks
			FROM nodes
			WHERE status = 'active'
			ORDER BY registered_at DESC
			LIMIT $1 OFFSET $2`,
			limit, offset,
		)
		if err != nil {
			log.Error("network nodes query", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
			return
		}
		defer rows.Close()

		nodes := []networkNodeRow{}
		for rows.Next() {
			var n networkNodeRow
			var registeredAt time.Time
			if err := rows.Scan(&n.NodeID, &n.Tier, &n.Status, &registeredAt, &n.Reliability, &n.TotalTasks); err != nil {
				log.Error("network nodes scan", zap.Error(err))
				continue
			}
			n.RegisteredAt = registeredAt.UTC().Format(time.RFC3339)
			nodes = append(nodes, n)
		}
		if err := rows.Err(); err != nil {
			log.Error("network nodes rows error", zap.Error(err))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(networkNodesResponse{Nodes: nodes, Total: total}) //nolint:errcheck
	}
}

// ── GET /v1/stake/requirements ───────────────────────────────────────────────

type tierRequirement struct {
	MinSats              int64   `json:"min_sats"`
	BaseRewardMultiplier float64 `json:"base_reward_multiplier"`
}

type stakeRequirementsResponse struct {
	Requirements map[string]tierRequirement `json:"requirements"`
}

// HandleGetStakeRequirements returns the static tier stake minimums.
func HandleGetStakeRequirements(log *zap.Logger) http.HandlerFunc {
	resp := stakeRequirementsResponse{
		Requirements: map[string]tierRequirement{
			"t1": {MinSats: stake.TierMinimums["t1"], BaseRewardMultiplier: 1.0},
			"t2": {MinSats: stake.TierMinimums["t2"], BaseRewardMultiplier: 2.5},
			"t3": {MinSats: stake.TierMinimums["t3"], BaseRewardMultiplier: 8.0},
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}

// ── GET /v1/stake/nodes/{node_id} ────────────────────────────────────────────

type stakeNodeResponse struct {
	NodeID             string  `json:"node_id"`
	ChannelID          string  `json:"channel_id"`
	ChannelCapacitySats int64  `json:"channel_capacity_sats"`
	LocalBalanceSats   int64   `json:"local_balance_sats"`
	TierMinimumSats    int64   `json:"tier_minimum_sats"`
	BonusMultiplier    float64 `json:"bonus_multiplier"`
	StakeStatus        string  `json:"stake_status"`
	OpenedAt           string  `json:"opened_at"`
	LastVerifiedAt     string  `json:"last_verified_at"`
}

// HandleGetStakeNode returns the stake record for a specific node.
func HandleGetStakeNode(db *pgxpool.Pool, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeIDStr := r.PathValue("node_id")
		if _, err := uuid.Parse(nodeIDStr); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid node_id"}) //nolint:errcheck
			return
		}

		var resp stakeNodeResponse
		resp.NodeID = nodeIDStr
		var openedAt, lastVerifiedAt time.Time

		err := db.QueryRow(r.Context(), `
			SELECT channel_id, channel_capacity, local_balance, tier_minimum,
			       bonus_multiplier, stake_status, opened_at, last_verified_at
			FROM node_stakes WHERE node_id = $1`,
			nodeIDStr,
		).Scan(
			&resp.ChannelID, &resp.ChannelCapacitySats, &resp.LocalBalanceSats,
			&resp.TierMinimumSats, &resp.BonusMultiplier, &resp.StakeStatus,
			&openedAt, &lastVerifiedAt,
		)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "node stake record not found"}) //nolint:errcheck
			return
		}
		resp.OpenedAt = openedAt.UTC().Format(time.RFC3339)
		resp.LastVerifiedAt = lastVerifiedAt.UTC().Format(time.RFC3339)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}
