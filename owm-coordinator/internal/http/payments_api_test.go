package poolhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

// mockPaymentRows implements pgx.Rows for testing.
type mockPaymentRows struct {
	payments []paymentRow
	err      error
	pos      int
}

func (m *mockPaymentRows) Next() bool {
	return m.pos < len(m.payments)
}

func (m *mockPaymentRows) Scan(dest ...any) error {
	if m.pos >= len(m.payments) {
		return pgx.ErrNoRows
	}
	p := m.payments[m.pos]
	m.pos++
	*dest[0].(*string) = p.TaskID
	*dest[1].(*string) = p.NodeID
	*dest[2].(*string) = p.NodeLNURI
	*dest[3].(*string) = p.TaskType
	*dest[4].(*string) = p.Status
	*dest[5].(*int) = p.RewardSats
	*dest[6].(*string) = p.SubmittedAt
	if dest[7] != nil {
		if ps, ok := dest[7].(**string); ok {
			*ps = p.CompletedAt
		}
	}
	return nil
}

func (m *mockPaymentRows) Err() error        { return m.err }
func (m *mockPaymentRows) Close()            {}
func (m *mockPaymentRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (m *mockPaymentRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockPaymentRows) Values() ([]any, error) { return nil, nil }
func (m *mockPaymentRows) RawValues() [][]byte   { return nil }
func (m *mockPaymentRows) Conn() *pgx.Conn      { return nil }

// mockPaymentsDB implements paymentsDB for testing.
type mockPaymentsDB struct {
	count    int
	payments []paymentRow
	countErr error
	queryErr error
}

func (m *mockPaymentsDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if m.countErr != nil {
		return &mockCountRow{err: m.countErr}
	}
	return &mockCountRow{total: m.count}
}

func (m *mockPaymentsDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return &mockPaymentRows{payments: m.payments}, nil
}

type mockCountRow struct {
	total int
	err   error
}

func (m *mockCountRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	*dest[0].(*int) = m.total
	return nil
}

func servePayments(req *http.Request, db *mockPaymentsDB) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/payments/history", handleGetPaymentsHistory(db, zap.NewNop()))
	mux.ServeHTTP(rr, req)
	return rr
}

func TestHandleGetPaymentsHistory_EmptyResult(t *testing.T) {
	db := &mockPaymentsDB{count: 0, payments: nil}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp paymentsHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Payments) != 0 {
		t.Errorf("expected 0 payments, got %d", len(resp.Payments))
	}
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
}

func TestHandleGetPaymentsHistory_WithPayments(t *testing.T) {
	completedAt1 := "2026-01-01T00:00:15Z"
	completedAt2 := "2026-01-02T00:00:10Z"
	db := &mockPaymentsDB{
		count: 2,
		payments: []paymentRow{
			{TaskID: "t1", NodeID: "n1", NodeLNURI: "ln://n1", TaskType: "inference", Status: "completed", RewardSats: 30, SubmittedAt: "2026-01-01T00:00:00Z", CompletedAt: &completedAt1},
			{TaskID: "t2", NodeID: "n2", NodeLNURI: "ln://n2", TaskType: "inference", Status: "completed", RewardSats: 25, SubmittedAt: "2026-01-02T00:00:00Z", CompletedAt: &completedAt2},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp paymentsHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Payments) != 2 {
		t.Errorf("expected 2 payments, got %d", len(resp.Payments))
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
}

func TestHandleGetPaymentsHistory_FilterByNodeID(t *testing.T) {
	completedAt := "2026-01-01T00:00:15Z"
	db := &mockPaymentsDB{
		count: 1,
		payments: []paymentRow{
			{TaskID: "t1", NodeID: "550e8400-e29b-41d4-a716-446655440000", NodeLNURI: "ln://n1", TaskType: "inference", Status: "completed", RewardSats: 30, SubmittedAt: "2026-01-01T00:00:00Z", CompletedAt: &completedAt},
		},
	}
	nodeID := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history?node_id="+nodeID, nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp paymentsHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Payments) != 1 {
		t.Errorf("expected 1 payment, got %d", len(resp.Payments))
	}
	if resp.Payments[0].NodeID != nodeID {
		t.Errorf("expected node_id %s, got %s", nodeID, resp.Payments[0].NodeID)
	}
}

func TestHandleGetPaymentsHistory_FilterByStatus(t *testing.T) {
	completedAt := "2026-01-01T00:00:15Z"
	db := &mockPaymentsDB{
		count: 1,
		payments: []paymentRow{
			{TaskID: "t1", NodeID: "n1", NodeLNURI: "ln://n1", TaskType: "inference", Status: "completed", RewardSats: 30, SubmittedAt: "2026-01-01T00:00:00Z", CompletedAt: &completedAt},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history?status=completed", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp paymentsHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Payments) != 1 {
		t.Errorf("expected 1 payment, got %d", len(resp.Payments))
	}
	if resp.Payments[0].Status != "completed" {
		t.Errorf("expected status completed, got %s", resp.Payments[0].Status)
	}
}

func TestHandleGetPaymentsHistory_InvalidStatus(t *testing.T) {
	db := &mockPaymentsDB{}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history?status=invalid_status", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid status") {
		t.Errorf("expected 'invalid status' error, got: %s", rr.Body.String())
	}
}

func TestHandleGetPaymentsHistory_InvalidNodeID(t *testing.T) {
	db := &mockPaymentsDB{}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history?node_id=not-a-uuid", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid node_id") {
		t.Errorf("expected 'invalid node_id' error, got: %s", rr.Body.String())
	}
}

func TestHandleGetPaymentsHistory_LimitOffset(t *testing.T) {
	completedAt := "2026-01-01T00:00:15Z"
	db := &mockPaymentsDB{count: 100, payments: []paymentRow{
		{TaskID: "t1", NodeID: "n1", NodeLNURI: "ln://n1", TaskType: "inference", Status: "completed", RewardSats: 30, SubmittedAt: "2026-01-01T00:00:00Z", CompletedAt: &completedAt},
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history?limit=10&offset=20", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp paymentsHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Total != 100 {
		t.Errorf("expected total 100, got %d", resp.Total)
	}
}

func TestHandleGetPaymentsHistory_DBError(t *testing.T) {
	db := &mockPaymentsDB{countErr: errors.New("connection reset")}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "internal error") {
		t.Errorf("expected 'internal error', got: %s", rr.Body.String())
	}
}

func TestHandleGetPaymentsHistory_QueryError(t *testing.T) {
	db := &mockPaymentsDB{count: 0, queryErr: errors.New("connection reset")}
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/history", nil)
	rr := servePayments(req, db)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}