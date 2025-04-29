package main

import (

	// Replace with actual path

	"context"
	"fmt"
	"log"
	"time"

	"kaspa_test/kaspa"

	"github.com/kaspanet/kaspad/infrastructure/network/rpcclient"
)

func main() {

	client, err := kaspa.NewClient(context.Background(), &kaspa.TestConfig)
	if err != nil {
		log.Fatal("error while establishing connection to DA layer: %w", err)
	}

	txHash, err := client.SubmitBlob([]byte{0x01})
	if err != nil {
		log.Fatal("Error sending %s", err)
	}
	fmt.Println("blob submitted", txHash)

}

func connectToRPC(rpcAddress string, timeout uint32) (*rpcclient.RPCClient, error) {

	rpcClient, err := rpcclient.NewRPCClient(rpcAddress)
	if err != nil {
		return nil, err
	}

	if timeout != 0 {
		rpcClient.SetTimeout(time.Duration(timeout) * time.Second)
	}

	return rpcClient, err
}
