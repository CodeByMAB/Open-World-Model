package poolhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// mockRow implements pgx.Row for testing.
type mockRow struct {
	nodeID string
	tier   string
	status string
	err    error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	*dest[0].(*string) = m.nodeID
	*dest[1].(*string) = m.tier
	*dest[2].(*string) = m.status
	return nil
}

// mockDB implements dbQuerier for testing.
type mockDB struct {
	nodeID string
	tier   string
	status string
	err    error
}

func (m *mockDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &mockRow{
		nodeID: m.nodeID,
		tier:   m.tier,
		status: m.status,
		err:    m.err,
	}
}

func serveWithMux(handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/pool/nodes/{pubkey}/status", handler)
	mux.ServeHTTP(rr, req)
	return rr
}

func TestHandleGetNodeStatus_Active(t *testing.T) {
	db := &mockDB{nodeID: "uuid-1", tier: "t1", status: "active"}
	handler := handleGetNodeStatus(db, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/internal/pool/nodes/somepubkey/status", nil)
	rr := serveWithMux(handler, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"status":"active"`) {
		t.Errorf("body missing status:active, got: %s", body)
	}
	if !strings.Contains(body, `"coordinator_verified":true`) {
		t.Errorf("body missing coordinator_verified:true, got: %s", body)
	}
}

func TestHandleGetNodeStatus_Pending(t *testing.T) {
	db := &mockDB{nodeID: "uuid-2", tier: "t1", status: "pending"}
	handler := handleGetNodeStatus(db, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/internal/pool/nodes/somepubkey/status", nil)
	rr := serveWithMux(handler, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"status":"pending"`) {
		t.Errorf("body missing status:pending, got: %s", rr.Body.String())
	}
}

func TestHandleGetNodeStatus_NotFound(t *testing.T) {
	db := &mockDB{err: pgx.ErrNoRows}
	handler := handleGetNodeStatus(db, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/internal/pool/nodes/somepubkey/status", nil)
	rr := serveWithMux(handler, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"error":"node not found"`) {
		t.Errorf("body missing error message, got: %s", rr.Body.String())
	}
}

func TestHandleGetNodeStatus_DBError(t *testing.T) {
	db := &mockDB{err: errors.New("connection reset by peer")}
	handler := handleGetNodeStatus(db, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/internal/pool/nodes/somepubkey/status", nil)
	rr := serveWithMux(handler, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"error":"internal error"`) {
		t.Errorf("body missing internal error message, got: %s", rr.Body.String())
	}
}
