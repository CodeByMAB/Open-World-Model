// Package lightning: CLNClient connects to Core Lightning via CLNRest (HTTP).
package lightning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CLNClient implements Client by calling Core Lightning's CLNRest API (/v1/{rpc_method}).
type CLNClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewCLNClient returns a CLN client. baseURL is e.g. "https://127.0.0.1:3010" (no trailing slash).
// apiKey is the node's CLN rune (sent as the Rune HTTP header per Core Lightning docs).
func NewCLNClient(baseURL, apiKey string) *CLNClient {
	return &CLNClient{
		baseURL: strings.TrimSuffix(strings.TrimSpace(baseURL), "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// clnReq POSTs a JSON object to /v1/{method}. CLNRest expects a plain JSON body, not JSON-RPC 2.0.
func (c *CLNClient) clnReq(ctx context.Context, method string, params interface{}, timeout time.Duration) ([]byte, error) {
	var body []byte
	var err error
	if params == nil {
		body = []byte("{}")
	} else {
		body, err = json.Marshal(params)
		if err != nil {
			return nil, err
		}
	}
	u := c.baseURL + "/v1/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Rune", c.apiKey)
	}
	client := c.httpClient
	if timeout != 0 {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return data, nil
	default:
		return nil, fmt.Errorf("cln %s: %s: %s", method, resp.Status, string(bytes.TrimSpace(data)))
	}
}

// clnDecodePayload unmarshals CLNRest JSON that may be either a bare RPC object
// or wrapped as {"result": ...}. Top-level JSON-RPC style errors become a Go error.
func clnDecodePayload(data []byte, dest interface{}) error {
	var wrap struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return json.Unmarshal(data, dest)
	}
	if wrap.Error != nil && wrap.Error.Message != "" {
		return fmt.Errorf("cln rpc error: %s", wrap.Error.Message)
	}
	if len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		return json.Unmarshal(wrap.Result, dest)
	}
	return json.Unmarshal(data, dest)
}

func parseMsatStringToSats(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "msat")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n / 1000
}

type clnChannelRow struct {
	ChannelID string `json:"channel_id"`
	PeerID    string `json:"peer_id"`
	State     string `json:"state"`
	// Older / alternate field names
	TotalSatoshis uint64 `json:"total_satoshis"`
	ToUsSatoshis  uint64 `json:"to_us_satoshis"`
	TotalMsat     string `json:"total_msat"`
	ToUsMsat      string `json:"to_us_msat"`
}

func clnRowToChannel(ch clnChannelRow) Channel {
	capSats := int64(ch.TotalSatoshis)
	localSats := int64(ch.ToUsSatoshis)
	if capSats == 0 && ch.TotalMsat != "" {
		capSats = parseMsatStringToSats(ch.TotalMsat)
	}
	if localSats == 0 && ch.ToUsMsat != "" {
		localSats = parseMsatStringToSats(ch.ToUsMsat)
	}
	active := strings.EqualFold(ch.State, "CHANNELD_NORMAL")
	return Channel{
		ChannelID:        ch.ChannelID,
		RemotePubkey:     ch.PeerID,
		CapacitySats:     capSats,
		LocalBalanceSats: localSats,
		Active:           active,
	}
}

// ListChannels calls listpeerchannels scoped to the peer pubkey when supported.
func (c *CLNClient) ListChannels(ctx context.Context, remotePubkeyHex string) ([]Channel, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	params := map[string]string{"id": remotePubkeyHex}
	data, err := c.clnReq(ctx, "listpeerchannels", params, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var flat struct {
		Channels []clnChannelRow `json:"channels"`
	}
	if err := clnDecodePayload(data, &flat); err != nil {
		return nil, err
	}
	if len(flat.Channels) > 0 {
		out := make([]Channel, 0, len(flat.Channels))
		for _, row := range flat.Channels {
			if normalizeNodePubkeyHex(row.PeerID) != normalizeNodePubkeyHex(remotePubkeyHex) {
				continue
			}
			out = append(out, clnRowToChannel(row))
		}
		return out, nil
	}

	var nested struct {
		Channels []struct {
			PeerID   string          `json:"peer_id"`
			Channels []clnChannelRow `json:"channels"`
		} `json:"channels"`
	}
	if err := clnDecodePayload(data, &nested); err != nil {
		return nil, err
	}
	want := normalizeNodePubkeyHex(remotePubkeyHex)
	var out []Channel
	for _, peer := range nested.Channels {
		if normalizeNodePubkeyHex(peer.PeerID) != want {
			continue
		}
		for _, row := range peer.Channels {
			row.PeerID = peer.PeerID
			out = append(out, clnRowToChannel(row))
		}
	}
	return out, nil
}

func normalizeCLNPayStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "complete", "completed", "succeeded", "success", "paid":
		return "SUCCEEDED"
	case "failed", "failure":
		return "FAILED"
	case "pending", "pending_state":
		return "IN_FLIGHT"
	default:
		if s == "" {
			return "IN_FLIGHT"
		}
		return strings.ToUpper(strings.TrimSpace(s))
	}
}

// SendPayment calls pay (keysend-style via destination + amount).
func (c *CLNClient) SendPayment(ctx context.Context, req SendPaymentRequest) (*PaymentResult, error) {
	timeout := time.Duration(req.TimeoutSecs) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"destination": req.DestPubkeyHex,
		"amount_msat": fmt.Sprintf("%dmsat", req.AmountSats*1000),
	}
	data, err := c.clnReq(ctx, "pay", params, timeout+10*time.Second)
	if err != nil {
		return nil, err
	}
	var pay struct {
		Result struct {
			PaymentHash     string `json:"payment_hash"`
			Status          string `json:"status"`
			PaymentPreimage string `json:"payment_preimage"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &pay); err != nil {
		return nil, err
	}
	if pay.Error != nil {
		return &PaymentResult{Status: "FAILED", FailureReason: pay.Error.Message}, nil
	}
	res := pay.Result
	if res.PaymentHash == "" && res.Status == "" && res.PaymentPreimage == "" {
		_ = json.Unmarshal(data, &res)
	}
	st := normalizeCLNPayStatus(res.Status)
	preimage := res.PaymentPreimage
	if preimage == "" && st == "SUCCEEDED" {
		preimage = "unknown"
	}
	return &PaymentResult{
		PaymentHash:   res.PaymentHash,
		Preimage:      preimage,
		FeeSats:       0,
		Status:        st,
		FailureReason: "",
	}, nil
}

// AddInvoice calls invoice.
func (c *CLNClient) AddInvoice(ctx context.Context, amountSats int64, memo string, expirySeconds int64) (*Invoice, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"amount_msat": fmt.Sprintf("%dmsat", amountSats*1000),
		"description": memo,
		"expiry":      expirySeconds,
	}
	data, err := c.clnReq(ctx, "invoice", params, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var inv struct {
		Bolt11      string `json:"bolt11"`
		PaymentHash string `json:"payment_hash"`
		Invoice     string `json:"invoice"`
	}
	if err := clnDecodePayload(data, &inv); err != nil {
		return nil, err
	}
	bolt := inv.Bolt11
	if bolt == "" {
		bolt = inv.Invoice
	}
	ph := inv.PaymentHash
	return &Invoice{
		PaymentRequest: bolt,
		PaymentHash:    ph,
		ExpiresAt:      time.Now().Unix() + expirySeconds,
	}, nil
}

// ForceCloseChan calls close with unilateraltimeout so the node performs a unilateral
// close after the timeout elapses (regtest-compatible; avoids unsupported "force").
func (c *CLNClient) ForceCloseChan(ctx context.Context, channelID string) error {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	_, err := c.clnReq(ctx, "close", map[string]interface{}{
		"id":                 channelID,
		"unilateraltimeout": 1,
	}, 120*time.Second)
	return err
}

// GetInfo calls getinfo.
func (c *CLNClient) GetInfo(ctx context.Context) (*NodeInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := c.clnReq(ctx, "getinfo", nil, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var info struct {
		ID          string      `json:"id"`
		Alias       string      `json:"alias"`
		BlockHeight interface{} `json:"blockheight"`
	}
	if err := clnDecodePayload(data, &info); err != nil {
		return nil, err
	}
	var bh uint32
	switch v := info.BlockHeight.(type) {
	case float64:
		bh = uint32(v)
	case json.Number:
		n, _ := v.Int64()
		bh = uint32(n)
	case string:
		n, _ := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
		bh = uint32(n)
	}
	return &NodeInfo{
		PubkeyHex:   info.ID,
		Alias:       info.Alias,
		BlockHeight: bh,
	}, nil
}

var _ Client = (*CLNClient)(nil)
