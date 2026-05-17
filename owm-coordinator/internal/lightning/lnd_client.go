// Package lightning: LNDClient connects to LND via gRPC with TLS and macaroon auth.
package lightning

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ltcsuite/lnd/lnrpc"
	"github.com/ltcsuite/lnd/lnrpc/routerrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// LNDClient implements Client by calling LND's gRPC API.
type LNDClient struct {
	conn         *grpc.ClientConn
	client       lnrpc.LightningClient
	routerClient routerrpc.RouterClient
	macaroonHex  string
}

// NewLNDClient dials LND at host with TLS from tlsCertPath and macaroon auth.
// macaroonBytes is the raw macaroon file content (hex or binary).
func NewLNDClient(host, tlsCertPath string, macaroonBytes []byte) (*LNDClient, error) {
	if len(macaroonBytes) > 0 && tlsCertPath == "" {
		return nil, fmt.Errorf("lightning.tls_cert_path is required when using LND macaroon auth: macaroons must not be sent over plaintext gRPC")
	}
	var opts []grpc.DialOption
	if tlsCertPath != "" {
		creds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
		if err != nil {
			return nil, fmt.Errorf("loading TLS cert: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	// Macaroon auth: use interceptors or metadata. LND expects hex-encoded macaroon in metadata.
	macHex := ""
	if len(macaroonBytes) > 0 {
		macHex = hexEncodeMacaroon(macaroonBytes)
		opts = append(opts, grpc.WithPerRPCCredentials(&macaroonCredential{macaroonHex: macHex}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, host, append(opts, grpc.WithBlock())...)
	if err != nil {
		return nil, fmt.Errorf("dialing LND: %w", err)
	}

	return &LNDClient{
		conn:         conn,
		client:       lnrpc.NewLightningClient(conn),
		routerClient: routerrpc.NewRouterClient(conn),
		macaroonHex:  macHex,
	}, nil
}

// macaroonCredential implements credentials.PerRPCCredentials.
type macaroonCredential struct {
	macaroonHex string
}

func (m *macaroonCredential) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	if m.macaroonHex == "" {
		return nil, nil
	}
	return map[string]string{"macaroon": m.macaroonHex}, nil
}

func (m *macaroonCredential) RequireTransportSecurity() bool { return true }

func hexEncodeMacaroon(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hex[c>>4]
		out[i*2+1] = hex[c&0x0f]
	}
	return string(out)
}

func normalizeNodePubkeyHex(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	return strings.ToLower(s)
}

// ListChannels returns channels whose remote pubkey matches remotePubkeyHex.
func (c *LNDClient) ListChannels(ctx context.Context, remotePubkeyHex string) ([]Channel, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	want := normalizeNodePubkeyHex(remotePubkeyHex)
	resp, err := c.client.ListChannels(ctx, &lnrpc.ListChannelsRequest{})
	if err != nil {
		return nil, err
	}
	var out []Channel
	for _, ch := range resp.Channels {
		if normalizeNodePubkeyHex(ch.RemotePubkey) != want {
			continue
		}
		chID := ch.ChannelPoint
		if chID == "" && ch.ChanId != 0 {
			chID = fmt.Sprintf("%d", ch.ChanId)
		}
		out = append(out, Channel{
			ChannelID:        chID,
			RemotePubkey:     ch.RemotePubkey,
			CapacitySats:     ch.Capacity,
			LocalBalanceSats: ch.LocalBalance,
			Active:           ch.Active,
		})
	}
	return out, nil
}

// SendPayment sends a payment via routerrpc.SendPaymentV2.
func (c *LNDClient) SendPayment(ctx context.Context, req SendPaymentRequest) (*PaymentResult, error) {
	timeout := req.TimeoutSecs
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout+10)*time.Second)
	defer cancel()

	stream, err := c.routerClient.SendPaymentV2(ctx, &routerrpc.SendPaymentRequest{
		Dest:           decodeHex(req.DestPubkeyHex),
		Amt:            req.AmountSats,
		TimeoutSeconds: int32(timeout),
	})
	if err != nil {
		return nil, err
	}
	for {
		update, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		if update.Status == lnrpc.Payment_SUCCEEDED {
			return &PaymentResult{
				PaymentHash:   fmt.Sprintf("%x", update.PaymentHash),
				Preimage:      fmt.Sprintf("%x", update.PaymentPreimage),
				FeeSats:       update.FeeSat,
				Status:        "SUCCEEDED",
				FailureReason: "",
			}, nil
		}
		if update.Status == lnrpc.Payment_FAILED {
			return &PaymentResult{
				PaymentHash:   fmt.Sprintf("%x", update.PaymentHash),
				Status:        "FAILED",
				FailureReason: update.FailureReason.String(),
			}, nil
		}
	}
}

// AddInvoice creates a BOLT11 invoice.
func (c *LNDClient) AddInvoice(ctx context.Context, amountSats int64, memo string, expirySeconds int64) (*Invoice, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := c.client.AddInvoice(ctx, &lnrpc.Invoice{
		Value:  amountSats,
		Memo:   memo,
		Expiry: expirySeconds,
	})
	if err != nil {
		return nil, err
	}
	paymentHash := ""
	if len(resp.RHash) > 0 {
		paymentHash = fmt.Sprintf("%x", resp.RHash)
	}
	return &Invoice{
		PaymentRequest: resp.PaymentRequest,
		PaymentHash:    paymentHash,
		ExpiresAt:      time.Now().Unix() + expirySeconds,
	}, nil
}

// ForceCloseChan force-closes the channel; polls for sweep to initiate.
// channelID should be "funding_txid:output_index" (LND ChannelPoint format).
func (c *LNDClient) ForceCloseChan(ctx context.Context, channelID string) error {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	parts := strings.SplitN(channelID, ":", 2)
	txidStr := parts[0]
	var outputIndex uint32
	if len(parts) == 2 {
		_, _ = fmt.Sscanf(parts[1], "%d", &outputIndex)
	}
	cp := &lnrpc.ChannelPoint{
		FundingTxid: &lnrpc.ChannelPoint_FundingTxidStr{FundingTxidStr: txidStr},
		OutputIndex: outputIndex,
	}
	stream, err := c.client.CloseChannel(ctx, &lnrpc.CloseChannelRequest{
		ChannelPoint: cp,
		Force:        true,
	})
	if err != nil {
		return err
	}
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// GetInfo returns node info.
func (c *LNDClient) GetInfo(ctx context.Context) (*NodeInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	info, err := c.client.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	return &NodeInfo{
		PubkeyHex:   info.IdentityPubkey,
		Alias:       info.Alias,
		BlockHeight: uint32(info.BlockHeight),
	}, nil
}

// Close closes the gRPC connection.
func (c *LNDClient) Close() error {
	return c.conn.Close()
}

func decodeHex(s string) []byte {
	if len(s)%2 != 0 {
		return nil
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var a, b byte
		switch {
		case s[i] >= '0' && s[i] <= '9':
			a = s[i] - '0'
		case s[i] >= 'a' && s[i] <= 'f':
			a = s[i] - 'a' + 10
		case s[i] >= 'A' && s[i] <= 'F':
			a = s[i] - 'A' + 10
		default:
			return nil
		}
		switch {
		case s[i+1] >= '0' && s[i+1] <= '9':
			b = s[i+1] - '0'
		case s[i+1] >= 'a' && s[i+1] <= 'f':
			b = s[i+1] - 'a' + 10
		case s[i+1] >= 'A' && s[i+1] <= 'F':
			b = s[i+1] - 'A' + 10
		default:
			return nil
		}
		out[i/2] = a<<4 | b
	}
	return out
}

var _ Client = (*LNDClient)(nil)
