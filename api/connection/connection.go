// Package connection manages gRPC connections to Fabric peers.
package connection

import (
	"crypto/x509"
	"fmt"
	"log"
	"os"

	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// PeerConfig holds the configuration needed to connect to a peer.
type PeerConfig struct {
	Endpoint     string // host:port
	HostOverride string // TLS server name
	TLSCertPath  string // TLS CA certificate of the peer organization
}

// NewGRPCConnection creates a new TLS gRPC connection to a Fabric peer.
// The connection is lazy: it is only dialed on the first request.
func NewGRPCConnection(cfg PeerConfig) (*grpc.ClientConn, error) {
	certPEM, err := os.ReadFile(cfg.TLSCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read TLS certificate '%s': %w", cfg.TLSCertPath, err)
	}
	cert, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TLS certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(cert)
	creds := credentials.NewClientTLSFromCert(certPool, cfg.HostOverride)

	conn, err := grpc.NewClient("dns:///"+cfg.Endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for '%s': %w", cfg.Endpoint, err)
	}
	log.Printf("gRPC client ready for peer %s", cfg.Endpoint)
	return conn, nil
}
