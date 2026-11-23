// Package gateway connects the API to Hyperledger Fabric through the Fabric Gateway.
// One gateway connection is opened per consortium organization, each signing with
// that organization's admin identity. This lets the demo prove that chaincode
// access control is enforced by MSP identity, not by the API.
package gateway

import (
	"fmt"
	"log"
	"strings"
	"time"

	"api/config"
	"api/connection"
	"api/identity"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/hash"
	"google.golang.org/grpc"
)

// OrgClient is a gateway connection signed by one organization.
type OrgClient struct {
	Config  config.OrgConfig
	conn    *grpc.ClientConn
	gateway *client.Gateway
	network *client.Network
}

// Gateway holds one OrgClient per organization.
type Gateway struct {
	cfg  *config.Config
	orgs map[string]*OrgClient
}

// New opens a gateway connection for every organization in the config.
func New(cfg *config.Config) (*Gateway, error) {
	g := &Gateway{cfg: cfg, orgs: map[string]*OrgClient{}}

	for _, orgCfg := range cfg.Orgs {
		conn, err := connection.NewGRPCConnection(connection.PeerConfig{
			Endpoint:     orgCfg.PeerEndpoint,
			HostOverride: orgCfg.PeerHostOverride,
			TLSCertPath:  orgCfg.TLSCACert,
		})
		if err != nil {
			g.Close()
			return nil, err
		}

		creds, err := identity.LoadFromFiles(orgCfg.CertPath, orgCfg.KeyPath, orgCfg.MSPID)
		if err != nil {
			conn.Close()
			g.Close()
			return nil, fmt.Errorf("failed to load identity of %s: %w", orgCfg.Name, err)
		}

		options := []client.ConnectOption{
			client.WithSign(creds.Signer),
			client.WithClientConnection(conn),
			client.WithEvaluateTimeout(10 * time.Second),
			client.WithEndorseTimeout(30 * time.Second),
			client.WithSubmitTimeout(10 * time.Second),
			client.WithCommitStatusTimeout(1 * time.Minute),
		}
		if creds.IsEd25519() {
			options = append(options, client.WithHash(hash.NONE))
		}

		gw, err := client.Connect(creds.Identity, options...)
		if err != nil {
			conn.Close()
			g.Close()
			return nil, fmt.Errorf("failed to connect gateway for %s: %w", orgCfg.Name, err)
		}

		g.orgs[orgCfg.Name] = &OrgClient{
			Config:  orgCfg,
			conn:    conn,
			gateway: gw,
			network: gw.GetNetwork(cfg.Channel),
		}
		log.Printf("Gateway connected as %s (%s) on channel %s", orgCfg.Name, orgCfg.MSPID, cfg.Channel)
	}
	return g, nil
}

// Org returns the client of an organization ("" = default organization).
func (g *Gateway) Org(name string) (*OrgClient, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = g.cfg.DefaultOrg
	}
	org, ok := g.orgs[name]
	if !ok {
		return nil, fmt.Errorf("unknown organization '%s'", name)
	}
	return org, nil
}

// Orgs returns the configuration of every connected organization.
func (g *Gateway) Orgs() []config.OrgConfig {
	return g.cfg.Orgs
}

// Contract returns a contract of a chaincode. Use contractName "" for the default contract.
func (o *OrgClient) Contract(chaincode, contractName string) *client.Contract {
	if contractName == "" {
		return o.network.GetContract(chaincode)
	}
	return o.network.GetContractWithName(chaincode, contractName)
}

// Close closes every gateway and gRPC connection.
func (g *Gateway) Close() {
	for name, org := range g.orgs {
		org.gateway.Close()
		org.conn.Close()
		log.Printf("Gateway connection of %s closed", name)
	}
}
