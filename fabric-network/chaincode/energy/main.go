package main

import (
	"energy/chaincode"
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func main() {
	// Both contracts share one world state.
	// "metering" is the demonstrated MVP scope (measurement, reconciliation, simulated bill).
	// "market" is the future P2P module, disabled unless the regulatory sandbox is enabled.
	metering := new(chaincode.MeteringContract)
	metering.Name = "metering"

	market := new(chaincode.MarketContract)
	market.Name = "market"

	energyChaincode, err := contractapi.NewChaincode(metering, market)
	if err != nil {
		log.Panicf("Error creating Energy chaincode: %v", err)
	}

	if err := energyChaincode.Start(); err != nil {
		log.Panicf("Error starting Energy chaincode: %v", err)
	}
}
