package poolhttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	poolhttp "github.com/owmnetwork/owm-coordinator/internal/http"
	"github.com/owmnetwork/owm-coordinator/internal/testutil"
)

func TestGetNetworkStats_Empty(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	h := poolhttp.HandleGetNetworkStats(pool, zap.NewNop())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/network/stats", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["active_nodes"]; !ok {
		t.Error("response missing active_nodes")
	}
	if _, ok := resp["active_nodes_by_tier"]; !ok {
		t.Error("response missing active_nodes_by_tier")
	}
}

func TestGetNetworkStats_WithNodes(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status, total_sats)
			 VALUES ($1, $2, $3, 't1', 'active', 1000)`,
			uuid.New(), "pk-stats-"+uuid.New().String()[:8], "uri@127.0.0.1:9735",
		)
		if err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	h := poolhttp.HandleGetNetworkStats(pool, zap.NewNop())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/network/stats", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := int(resp["active_nodes"].(float64)); got != 3 {
		t.Errorf("active_nodes: got %d, want 3", got)
	}
}

func TestGetNetworkNodes_Pagination(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
			 VALUES ($1, $2, $3, 't1', 'active')`,
			uuid.New(), "pk-nodes-"+uuid.New().String()[:8], "uri@127.0.0.1:9735",
		)
		if err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	h := poolhttp.HandleGetNetworkNodes(pool, zap.NewNop())

	// Page 1: limit 3
	req := httptest.NewRequest(http.MethodGet, "/v1/network/nodes?limit=3", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var resp struct {
		Nodes []map[string]any `json:"nodes"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Nodes) != 3 {
		t.Errorf("nodes len: got %d, want 3", len(resp.Nodes))
	}
	if resp.Total != 5 {
		t.Errorf("total: got %d, want 5", resp.Total)
	}
	// Verify no private fields exposed.
	for _, n := range resp.Nodes {
		if _, ok := n["public_key"]; ok {
			t.Error("public_key must not appear in network/nodes response")
		}
		if _, ok := n["ln_node_uri"]; ok {
			t.Error("ln_node_uri must not appear in network/nodes response")
		}
	}
}

func TestGetStakeRequirements(t *testing.T) {
	h := poolhttp.HandleGetStakeRequirements(zap.NewNop())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/stake/requirements", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var resp struct {
		Requirements map[string]struct {
			MinSats int64 `json:"min_sats"`
		} `json:"requirements"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Requirements["t1"].MinSats != 100_000 {
		t.Errorf("t1 min_sats: got %d, want 100000", resp.Requirements["t1"].MinSats)
	}
	if resp.Requirements["t2"].MinSats != 500_000 {
		t.Errorf("t2 min_sats: got %d, want 500000", resp.Requirements["t2"].MinSats)
	}
	if resp.Requirements["t3"].MinSats != 2_000_000 {
		t.Errorf("t3 min_sats: got %d, want 2000000", resp.Requirements["t3"].MinSats)
	}
}

func TestGetStakeNode_NotFound(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	h := poolhttp.HandleGetStakeNode(pool, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/stake/nodes/"+uuid.New().String(), nil)
	req.SetPathValue("node_id", uuid.New().String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

func TestGetStakeNode_Found(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status)
		 VALUES ($1, $2, $3, 't1', 'active')`,
		nodeID, "pk-stake-"+nodeID.String()[:8], nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance, tier_minimum, stake_status)
		 VALUES ($1, 'ch-test', 200000, 150000, 100000, 'active')`,
		nodeID,
	)
	if err != nil {
		t.Fatalf("insert node_stakes: %v", err)
	}

	h := poolhttp.HandleGetStakeNode(pool, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/stake/nodes/"+nodeID.String(), nil)
	req.SetPathValue("node_id", nodeID.String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := int64(resp["channel_capacity_sats"].(float64)); got != 200_000 {
		t.Errorf("channel_capacity_sats: got %d, want 200000", got)
	}
}
