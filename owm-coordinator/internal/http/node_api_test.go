package poolhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	poolhttp "github.com/owmnetwork/owm-coordinator/internal/http"
	"github.com/owmnetwork/owm-coordinator/internal/testutil"
)

func TestGetNodeStatusV1_NotFound(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	h := poolhttp.HandleGetNodeStatusV1(pool, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes/"+uuid.New().String()+"/status", nil)
	req.SetPathValue("node_id", uuid.New().String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

func TestGetNodeStatusV1_Found(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status, reliability, total_tasks, total_sats)
		 VALUES ($1, $2, $3, 't2', 'active', 0.95, 42, 500000)`,
		nodeID, "pk-status-"+nodeID.String()[:8], nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	h := poolhttp.HandleGetNodeStatusV1(pool, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes/"+nodeID.String()+"/status", nil)
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
	if resp["tier"] != "t2" {
		t.Errorf("tier: got %q, want t2", resp["tier"])
	}
	if resp["status"] != "active" {
		t.Errorf("status: got %q, want active", resp["status"])
	}
	if int(resp["total_tasks_completed"].(float64)) != 42 {
		t.Errorf("total_tasks_completed: got %v, want 42", resp["total_tasks_completed"])
	}
}

func TestGetNodeStatusV1_InvalidUUID(t *testing.T) {
	pool := testutil.MustDB(t)

	h := poolhttp.HandleGetNodeStatusV1(pool, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes/not-a-uuid/status", nil)
	req.SetPathValue("node_id", "not-a-uuid")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

func TestGetNodeEarnings_Found(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	ctx := context.Background()
	nodeID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO nodes (node_id, public_key, ln_node_uri, tier, status, total_tasks, total_sats)
		 VALUES ($1, $2, $3, 't1', 'active', 2, 250)`,
		nodeID, "pk-earn-"+nodeID.String()[:8], nodeID.String()+"@127.0.0.1:9735",
	)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	for i := 0; i < 2; i++ {
		_, err = pool.Exec(ctx,
			`INSERT INTO tasks (task_id, task_type, assigned_node, status, reward_sats, timeout_seconds)
			 VALUES ($1, 'inference', $2, 'completed', 125, 60)`,
			uuid.New(), nodeID,
		)
		if err != nil {
			t.Fatalf("insert task: %v", err)
		}
	}

	h := poolhttp.HandleGetNodeEarnings(pool, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes/"+nodeID.String()+"/earnings", nil)
	req.SetPathValue("node_id", nodeID.String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var resp struct {
		TotalSatsEarned int64            `json:"total_sats_earned"`
		Tasks           []map[string]any `json:"tasks"`
		Total           int              `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalSatsEarned != 250 {
		t.Errorf("total_sats_earned: got %d, want 250", resp.TotalSatsEarned)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("tasks len: got %d, want 2", len(resp.Tasks))
	}
}

func TestGetNodeEarnings_NotFound(t *testing.T) {
	pool := testutil.MustDB(t)
	testutil.TruncateAll(t, pool)

	h := poolhttp.HandleGetNodeEarnings(pool, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes/"+uuid.New().String()+"/earnings", nil)
	req.SetPathValue("node_id", uuid.New().String())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}
