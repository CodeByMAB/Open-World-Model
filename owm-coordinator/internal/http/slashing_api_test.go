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

// mockSlashingRow implements pgx.Row for testing.
type mockSlashingRow struct {
	eventID           string
	nodeID            string
	channelID         string
	reason            string
	evidence          string
	slashedAt         string
	cooldownExpiresAt string
	total             int
	err               error
}

func (m *mockSlashingRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if len(dest) == 1 {
		// Count query - single integer
		*dest[0].(*int) = m.total
		return nil
	}
	*dest[0].(*string) = m.eventID
	*dest[1].(*string) = m.nodeID
	*dest[2].(*string) = m.channelID
	*dest[3].(*string) = m.reason
	*dest[4].(*string) = m.evidence
	*dest[5].(*string) = m.slashedAt
	*dest[6].(*string) = m.cooldownExpiresAt
	return nil
}

// mockSlashingRows implements pgx.Rows for testing.
type mockSlashingRows struct {
	rows []slashingEventRow
	err  error
	pos  int
}

func (m *mockSlashingRows) Next() bool {
	return m.pos < len(m.rows)
}

func (m *mockSlashingRows) Scan(dest ...any) error {
	if m.pos >= len(m.rows) {
		return pgx.ErrNoRows
	}
	row := m.rows[m.pos]
	m.pos++
	*dest[0].(*string) = row.EventID
	*dest[1].(*string) = row.NodeID
	*dest[2].(*string) = row.ChannelID
	*dest[3].(*string) = row.Reason
	*dest[4].(*string) = row.Evidence
	*dest[5].(*string) = row.SlashedAt
	*dest[6].(*string) = row.CooldownExpiresAt
	return nil
}

func (m *mockSlashingRows) Err() error        { return m.err }
func (m *mockSlashingRows) Close()            {}
func (m *mockSlashingRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (m *mockSlashingRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockSlashingRows) Values() ([]any, error) { return nil, nil }
func (m *mockSlashingRows) RawValues() [][]byte   { return nil }
func (m *mockSlashingRows) Conn() *pgx.Conn       { return nil }

// mockSlashingDB implements slashingLogDB for testing.
type mockSlashingDB struct {
	count      int
	rows       []slashingEventRow
	countErr   error
	queryErr   error
	rowsErr    error
	nodeID     string // if set, validate node_id param
}

func (m *mockSlashingDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if m.countErr != nil {
		return &mockSlashingRow{err: m.countErr}
	}
	if len(args) > 0 {
		m.nodeID = args[0].(string)
	}
	return &mockSlashingRow{total: m.count}
}

func (m *mockSlashingDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return &mockSlashingRows{rows: m.rows, err: m.rowsErr}, nil
}

func serveSlashing(req *http.Request, db *mockSlashingDB) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/network/slashing-log", handleGetSlashingLog(db, zap.NewNop()))
	mux.ServeHTTP(rr, req)
	return rr
}

func TestHandleGetSlashingLog_EmptyResult(t *testing.T) {
	db := &mockSlashingDB{count: 0, rows: nil}
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log", nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp slashingLogResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(resp.Events))
	}
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
}

func TestHandleGetSlashingLog_WithEvents(t *testing.T) {
	db := &mockSlashingDB{
		count: 2,
		rows: []slashingEventRow{
			{EventID: "e1", NodeID: "n1", ChannelID: "ch1", Reason: "offline", Evidence: "ev1", SlashedAt: "2026-01-01T00:00:00Z", CooldownExpiresAt: "2026-01-31T00:00:00Z"},
			{EventID: "e2", NodeID: "n2", ChannelID: "ch2", Reason: "misbehavior", Evidence: "ev2", SlashedAt: "2026-01-02T00:00:00Z", CooldownExpiresAt: "2026-02-01T00:00:00Z"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log", nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp slashingLogResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(resp.Events))
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
}

func TestHandleGetSlashingLog_FilterByNodeID(t *testing.T) {
	db := &mockSlashingDB{
		count: 1,
		rows: []slashingEventRow{
			{EventID: "e1", NodeID: "550e8400-e29b-41d4-a716-446655440000", ChannelID: "ch1", Reason: "offline", Evidence: "ev1", SlashedAt: "2026-01-01T00:00:00Z", CooldownExpiresAt: "2026-01-31T00:00:00Z"},
		},
	}
	nodeID := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log?node_id="+nodeID, nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp slashingLogResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(resp.Events))
	}
	if resp.Events[0].NodeID != nodeID {
		t.Errorf("expected node_id %s, got %s", nodeID, resp.Events[0].NodeID)
	}
}

func TestHandleGetSlashingLog_InvalidUUID(t *testing.T) {
	db := &mockSlashingDB{}
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log?node_id=not-a-uuid", nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid node_id") {
		t.Errorf("expected 'invalid node_id' error, got: %s", rr.Body.String())
	}
}

func TestHandleGetSlashingLog_LimitOffset(t *testing.T) {
	db := &mockSlashingDB{count: 100, rows: []slashingEventRow{
		{EventID: "e1", NodeID: "n1", ChannelID: "ch1", Reason: "r", Evidence: "e", SlashedAt: "2026-01-01T00:00:00Z", CooldownExpiresAt: "2026-01-31T00:00:00Z"},
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log?limit=10&offset=20", nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp slashingLogResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// With a real DB, limit=10 would return at most 10 rows; mock returns 1
	if resp.Total != 100 {
		t.Errorf("expected total 100, got %d", resp.Total)
	}
}

func TestHandleGetSlashingLog_LimitCapped(t *testing.T) {
	db := &mockSlashingDB{count: 300, rows: nil}
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log?limit=500", nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// Limit should be capped at 200; total is 300 but with limit=200
	var resp slashingLogResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Total != 300 {
		t.Errorf("expected total 300, got %d", resp.Total)
	}
}

func TestHandleGetSlashingLog_DBError(t *testing.T) {
	db := &mockSlashingDB{countErr: errors.New("connection reset")}
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log", nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "internal error") {
		t.Errorf("expected 'internal error', got: %s", rr.Body.String())
	}
}

func TestHandleGetSlashingLog_QueryError(t *testing.T) {
	db := &mockSlashingDB{count: 0, queryErr: errors.New("connection reset")}
	req := httptest.NewRequest(http.MethodGet, "/v1/network/slashing-log", nil)
	rr := serveSlashing(req, db)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}