package lightning

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLNClient_GetInfo_201Wrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/getinfo" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Rune"); got != "test-rune" {
			t.Errorf("Rune header %q", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"result":{"id":"02abcd","alias":"n1","blockheight":123}}`)
	}))
	defer srv.Close()

	c := NewCLNClient(srv.URL, "test-rune")
	info, err := c.GetInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.PubkeyHex != "02abcd" || info.Alias != "n1" || info.BlockHeight != 123 {
		t.Fatalf("GetInfo: %+v", info)
	}
}

func TestCLNClient_ListChannels_FlatMsat(t *testing.T) {
	body := `{"channels":[{"channel_id":"ch1","peer_id":"02ABCD","state":"CHANNELD_NORMAL","total_msat":"3000000000msat","to_us_msat":"2500000000msat"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/listpeerchannels" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewCLNClient(srv.URL, "r")
	chans, err := c.ListChannels(context.Background(), "02abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 1 {
		t.Fatalf("len=%d", len(chans))
	}
	if chans[0].ChannelID != "ch1" || chans[0].CapacitySats != 3_000_000 || chans[0].LocalBalanceSats != 2_500_000 || !chans[0].Active {
		t.Fatalf("channel: %+v", chans[0])
	}
}

func TestCLNClient_SendPayment_CompleteStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"payment_hash":"abc","status":"complete","payment_preimage":"010203"}`)
	}))
	defer srv.Close()

	c := NewCLNClient(srv.URL, "r")
	res, err := c.SendPayment(context.Background(), SendPaymentRequest{
		DestPubkeyHex: "02aa",
		AmountSats:    1000,
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "SUCCEEDED" || res.PaymentHash != "abc" || res.Preimage != "010203" {
		t.Fatalf("%+v", res)
	}
}

func TestCLNClient_AddInvoice_Flat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"bolt11":"lnbc1demo","payment_hash":"deadbeef"}`)
	}))
	defer srv.Close()

	c := NewCLNClient(srv.URL, "r")
	inv, err := c.AddInvoice(context.Background(), 1000, "m", 3600)
	if err != nil {
		t.Fatal(err)
	}
	if inv.PaymentRequest != "lnbc1demo" || inv.PaymentHash != "deadbeef" {
		t.Fatalf("%+v", inv)
	}
}

func TestCLNClient_ForceCloseChan(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/close" {
			t.Errorf("path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c := NewCLNClient(srv.URL, "r")
	if err := c.ForceCloseChan(context.Background(), "chan42"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, `"unilateraltimeout":1`) {
		t.Fatalf("expected unilateraltimeout=1 in body, got %q", gotBody)
	}
	if strings.Contains(gotBody, `"force"`) {
		t.Fatalf("unexpected force flag in body %q", gotBody)
	}
}

func TestCLNClient_ForceCloseChan_CLNErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"Unknown JSON parameters: force"}}`)
	}))
	defer srv.Close()

	c := NewCLNClient(srv.URL, "r")
	err := c.ForceCloseChan(context.Background(), "chan42")
	if err == nil {
		t.Fatal("expected error from CLN error payload")
	}
	if !strings.Contains(err.Error(), "close") {
		t.Fatalf("expected close in error, got %v", err)
	}
}
