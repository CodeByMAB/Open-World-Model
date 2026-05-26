package poolhttp

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/stake"
)

type slashAckRequest struct {
	NodeID           string `json:"node_id"`
	Tier             string `json:"tier"`
	ReasonHash       string `json:"reason_hash"`
	MaintainerPubKey string `json:"maintainer_pubkey"`
	Signature        string `json:"signature"`
}

type slashAckResponse struct {
	Status   string `json:"status"`
	AckCount int    `json:"ack_count,omitempty"`
}

// HandleMaintainerSlashAck processes a signed core-maintainer acknowledgment
// for a pending T2/T3 slash decision (SRS-SEC-14).
func HandleMaintainerSlashAck(verif *stake.Verifier, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req slashAckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"}) //nolint:errcheck
			return
		}

		if req.NodeID == "" || req.Tier == "" || req.ReasonHash == "" ||
			req.MaintainerPubKey == "" || req.Signature == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "node_id, tier, reason_hash, maintainer_pubkey, and signature are required"}) //nolint:errcheck
			return
		}

		nodeID, err := uuid.Parse(req.NodeID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid node_id"}) //nolint:errcheck
			return
		}

		if err := verif.RecordMaintainerAck(r.Context(), nodeID, req.Tier,
			req.MaintainerPubKey, req.Signature, req.ReasonHash); err != nil {
			log.Warn("maintainer ack rejected", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) //nolint:errcheck
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(slashAckResponse{Status: "recorded"}) //nolint:errcheck
	}
}
