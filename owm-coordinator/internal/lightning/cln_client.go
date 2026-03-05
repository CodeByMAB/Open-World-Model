// Package lightning: CLNClient connects to Core Lightning via REST API.
package lightning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CLNClient implements Client by calling Core Lightning's REST API.
type CLNClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewCLNClient returns a CLN client. baseURL is e.g. "https://cln:9745", apiKey for Bearer auth.
func NewCLNClient(baseURL, apiKey string) *CLNClient {
	return &CLNClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// clnReq sends a JSON-RPC POST to /v1/{method}.
func (c *CLNClient) clnReq(ctx context.Context, method string, params interface{}, timeout time.Duration) ([]byte, error) {
	body := map[string]interface{}{"jsonrpc": "2.0", "id": "owm", "method": method}
	if params != nil {
		body["params"] = params
	}
	enc, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/"+method, bytes.NewReader(enc))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cln %s: %s", method, resp.Status)
	}
	return data, nil
}

// ListChannels calls listpeerchannels and filters by peer_id (hex).
func (c *CLNClient) ListChannels(ctx context.Context, remotePubkeyHex string) ([]Channel, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := c.clnReq(ctx, "listpeerchannels", map[string]string{"id": remotePubkeyHex}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var out struct {
		Result struct {
			Channels []struct {
				ChannelID     string `json:"channel_id"`
				PeerID        string `json:"peer_id"`
				TotalSatoshis uint64 `json:"total_satoshis"`
				ToUsSatoshis  uint64 `json:"to_us_satoshis"`
				State         string `json:"state"`
			} `json:"channels"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	var chans []Channel
	for _, ch := range out.Result.Channels {
		active := ch.State == "CHANNELD_NORMAL"
		chans = append(chans, Channel{
			ChannelID:        ch.ChannelID,
			RemotePubkey:     ch.PeerID,
			CapacitySats:     int64(ch.TotalSatoshis),
			LocalBalanceSats: int64(ch.ToUsSatoshis),
			Active:           active,
		})
	}
	return chans, nil
}

// SendPayment calls pay (BOLT11 or keysend by dest + amount).
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
			PaymentHash   string `json:"payment_hash"`
			Status        string `json:"status"`
			PaymentPreimage string `json:"payment_preimage"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal(data, &pay); err != nil {
		return nil, err
	}
	if pay.Error != nil {
		return &PaymentResult{Status: "FAILED", FailureReason: pay.Error.Message}, nil
	}
	preimage := pay.Result.PaymentPreimage
	if preimage == "" {
		preimage = "unknown"
	}
	return &PaymentResult{
		PaymentHash:   pay.Result.PaymentHash,
		Preimage:      preimage,
		FeeSats:       0,
		Status:        pay.Result.Status,
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
		Result struct {
			Bolt11    string `json:"bolt11"`
			PaymentHash string `json:"payment_hash"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, err
	}
	return &Invoice{
		PaymentRequest: inv.Result.Bolt11,
		PaymentHash:    inv.Result.PaymentHash,
		ExpiresAt:      time.Now().Unix() + expirySeconds,
	}, nil
}

// ForceCloseChan calls close with force.
func (c *CLNClient) ForceCloseChan(ctx context.Context, channelID string) error {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	_, err := c.clnReq(ctx, "close", map[string]string{"id": channelID, "force": "true"}, 120*time.Second)
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
		Result struct {
			ID     string `json:"id"`
			Alias  string `json:"alias"`
			BlockHeight uint32 `json:"blockheight"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &NodeInfo{
		PubkeyHex:   info.Result.ID,
		Alias:       info.Result.Alias,
		BlockHeight: info.Result.BlockHeight,
	}, nil
}

var _ Client = (*CLNClient)(nil)
