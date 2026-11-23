package main

import (
	"governance/chaincode"
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func main() {
	governanceChaincode, err := contractapi.NewChaincode(new(chaincode.GovernanceChaincode))
	if err != nil {
		log.Panicf("Error creating Governance chaincode: %v", err)
	}

	if err := governanceChaincode.Start(); err != nil {
		log.Panicf("Error starting Governance chaincode: %v", err)
	}
}
