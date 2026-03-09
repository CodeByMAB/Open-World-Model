package rpc_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	coordinatorv1 "github.com/owmnetwork/owm-coordinator/proto/coordinator/v1"
)

type noopServer struct {
	coordinatorv1.UnimplementedCoordinatorServiceServer
}

func TestGRPCServer_RejectsClientWithoutCert(t *testing.T) {
	// Step 1 — Generate CA key and self-signed CA cert in memory.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate CA serial: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	// Step 2 — Generate server key and server cert signed by the CA.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate server serial: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("marshal server key: %v", err)
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	// Step 3 — Build CA cert pool and server tls.Config.
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)

	serverTLSCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("load server TLS key pair: %v", err)
	}
	serverTLSCfg := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		Certificates: []tls.Certificate{serverTLSCert},
		MinVersion:   tls.VersionTLS13,
	}

	// Step 4 — Start a real gRPC server with mTLS.
	grpcSrv := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLSCfg)))
	coordinatorv1.RegisterCoordinatorServiceServer(grpcSrv, &noopServer{})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go grpcSrv.Serve(lis) //nolint:errcheck
	defer grpcSrv.Stop()

	// Step 5 — Attempt to connect with a plain TLS client (no client cert).
	clientTLSCfg := &tls.Config{
		RootCAs:    caPool,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS13,
		// Intentionally no Certificates field — this client presents no cert.
	}
	conn, err := grpc.Dial( //nolint:staticcheck
		lis.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLSCfg)),
	)
	if err != nil {
		t.Fatalf("grpc.Dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := coordinatorv1.NewCoordinatorServiceClient(conn)
	_, rpcErr := client.GetCurrentModel(ctx, &coordinatorv1.GetModelRequest{})

	// Step 6 — Assert TLS handshake failure.
	if rpcErr == nil {
		t.Fatal("expected RPC to fail because client presented no certificate, but it succeeded")
	}

	st, ok := status.FromError(rpcErr)
	if !ok {
		t.Fatalf("expected a gRPC status error, got: %v", rpcErr)
	}

	// codes.Unimplemented means the server accepted the client and dispatched
	// the call — mTLS was not enforced.
	if st.Code() == codes.Unimplemented {
		t.Fatalf("got Unimplemented — mTLS was not enforced; server accepted the unauthenticated client: %v", rpcErr)
	}

	// The server must close the connection at the TLS layer, which surfaces as
	// codes.Unavailable on the client side.
	if st.Code() != codes.Unavailable {
		t.Fatalf("expected Unavailable (TLS handshake rejection), got %v: %v", st.Code(), rpcErr)
	}

	// The error message should reference the TLS/handshake/certificate failure.
	errMsg := strings.ToLower(rpcErr.Error())
	if !strings.Contains(errMsg, "handshake") && !strings.Contains(errMsg, "certificate") && !strings.Contains(errMsg, "tls") {
		t.Fatalf("expected error to mention TLS/handshake/certificate, got: %v", rpcErr)
	}

	t.Logf("got expected error (mTLS rejection): %v", rpcErr)
}
