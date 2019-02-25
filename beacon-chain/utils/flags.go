package utils

import (
	"github.com/urfave/cli"
)

var (
	// DemoConfigFlag determines whether to launch a beacon chain using demo parameters
	// such as shorter cycle length, fewer shards, and more.
	DemoConfigFlag = cli.BoolFlag{
		Name:  "demo-config",
		Usage: " Run the beacon node using demo paramteres (i.e. shorter cycles, fewer shards and committees)",
	}
	// Web3ProviderFlag defines a flag for a mainchain RPC endpoint.
	Web3ProviderFlag = cli.StringFlag{
		Name:  "web3provider",
		Usage: "A mainchain web3 provider string endpoint. Can either be an IPC file string or a WebSocket endpoint. Uses WebSockets by default at ws://127.0.0.1:8546. Cannot be an HTTP endpoint.",
		Value: "ws://127.0.0.1:8546",
	}
	// DepositContractFlag defines a flag for the deposit contract address.
	DepositContractFlag = cli.StringFlag{
		Name:  "deposit-contract",
		Usage: "Deposit contract address. Beacon chain node will listen logs coming from the deposit contract to determine when validator is eligible to participate.",
	}
	// RPCPort defines a beacon node RPC port to open.
	RPCPort = cli.StringFlag{
		Name:  "rpc-port",
		Usage: "RPC port exposed by a beacon node",
		Value: "4000",
	}
	// CertFlag defines a flag for the node's TLS certificate.
	CertFlag = cli.StringFlag{
		Name:  "tls-cert",
		Usage: "Certificate for secure gRPC. Pass this and the tls-key flag in order to use gRPC securely.",
	}
	// KeyFlag defines a flag for the node's TLS key.
	KeyFlag = cli.StringFlag{
		Name:  "tls-key",
		Usage: "Key for secure gRPC. Pass this and the tls-cert flag in order to use gRPC securely.",
	}
	// GenesisJSON defines a flag for bootstrapping validators from genesis JSON.
	// If this flag is not specified, beacon node will bootstrap validators from code from crystallized_state.go.
	GenesisJSON = cli.StringFlag{
		Name:  "genesis-json",
		Usage: "Beacon node will bootstrap genesis state defined in genesis.json",
	}
	// EnablePOWChain tells the beacon node to use a real web3 endpoint. Disabled by default.
	EnablePOWChain = cli.BoolFlag{
		Name:  "enable-powchain",
		Usage: "Enable a real, web3 proof-of-work chain endpoint in the beacon node",
	}
	// EnableDBCleanup tells the beacon node to automatically clean DB content such as block vote cache.
	EnableDBCleanup = cli.BoolFlag{
		Name:  "enable-db-cleanup",
		Usage: "Enable automatic DB cleanup routine",
	}
	// ChainStartDelay tells the beacon node to wait for a period of time from the current time, before
	// logging chainstart.
	ChainStartDelay = cli.Uint64Flag{
		Name:  "chain-start-delay",
		Usage: "Delay the chain start so as to make local testing easier",
	}
)
