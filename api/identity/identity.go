// Package identity handles Fabric identity creation and signing operations.
package identity

import (
	"crypto/x509"
	"fmt"
	"log"
	"os"

	"github.com/hyperledger/fabric-gateway/pkg/identity"
)

// Credentials holds the identity and signing components for Fabric authentication.
type Credentials struct {
	Identity    identity.Identity
	Signer      identity.Sign
	Certificate *x509.Certificate
}

// LoadFromFiles creates identity credentials from certificate and key files.
func LoadFromFiles(certPath, keyPath, mspID string) (*Credentials, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate from '%s': %w", certPath, err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key from '%s': %w", keyPath, err)
	}

	return LoadFromPEM(certPEM, keyPEM, mspID)
}

// LoadFromPEM creates identity credentials from PEM-encoded certificate and key.
func LoadFromPEM(certPEM, keyPEM []byte, mspID string) (*Credentials, error) {
	cert, err := parseCertificate(certPEM)
	if err != nil {
		return nil, err
	}

	id, err := createIdentity(mspID, cert)
	if err != nil {
		return nil, err
	}

	signer, err := createSigner(keyPEM)
	if err != nil {
		return nil, err
	}

	log.Printf("Identity loaded successfully for MSP: %s", mspID)

	return &Credentials{
		Identity:    id,
		Signer:      signer,
		Certificate: cert,
	}, nil
}

// parseCertificate parses an X.509 certificate from PEM format.
func parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	cert, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	return cert, nil
}

// createIdentity creates a Fabric identity from MSP ID and certificate.
func createIdentity(mspID string, cert *x509.Certificate) (identity.Identity, error) {
	id, err := identity.NewX509Identity(mspID, cert)
	if err != nil {
		return nil, fmt.Errorf("failed to create X509 identity for MSP '%s': %w", mspID, err)
	}
	return id, nil
}

// createSigner creates a signing function from a private key.
func createSigner(keyPEM []byte) (identity.Sign, error) {
	privateKey, err := identity.PrivateKeyFromPEM(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	signer, err := identity.NewPrivateKeySign(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	return signer, nil
}

// IsEd25519 checks if the certificate uses the Ed25519 algorithm.
func (c *Credentials) IsEd25519() bool {
	return c.Certificate.PublicKeyAlgorithm == x509.Ed25519
}
