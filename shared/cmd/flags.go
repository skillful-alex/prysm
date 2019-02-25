// Package cmd defines the command line flags for the shared utlities.
package cmd

import (
	"github.com/urfave/cli"
)

var (
	// VerbosityFlag defines the logrus configuration.
	VerbosityFlag = cli.StringFlag{
		Name:  "verbosity",
		Usage: "Logging verbosity (debug, info=default, warn, error, fatal, panic)",
		Value: "info",
	}
	// DataDirFlag defines a path on disk.
	DataDirFlag = DirectoryFlag{
		Name:  "datadir",
		Usage: "Data directory for the databases and keystore",
		Value: DirectoryString{DefaultDataDir()},
	}
	// EnableTracingFlag defines a flag to enable p2p message tracing.
	EnableTracingFlag = cli.BoolFlag{
		Name:  "enable-tracing",
		Usage: "Enable request tracing.",
	}
	// TracingEndpointFlag flag defines the http endpoint for serving traces to Jaeger.
	TracingEndpointFlag = cli.StringFlag{
		Name:  "tracing-endpoint",
		Usage: "Tracing endpoint defines where beacon chain traces are exposed to Jaeger.",
		Value: "http://127.0.0.1:14268",
	}
	// TraceSampleFractionFlag defines a flag to indicate what fraction of p2p
	// messages are sampled for tracing.
	TraceSampleFractionFlag = cli.Float64Flag{
		Name:  "trace-sample-fraction",
		Usage: "Indicate what fraction of p2p messages are sampled for tracing.",
		Value: 0.20,
	}
	// DisableMonitoringFlag defines a flag to disable the metrics collection.
	DisableMonitoringFlag = cli.BoolFlag{
		Name:  "disable-monitoring",
		Usage: "Disable monitoring service.",
	}
	// MonitoringPortFlag defines the http port used to serve prometheus metrics.
	MonitoringPortFlag = cli.Int64Flag{
		Name:  "monitoring-port",
		Usage: "Port used to listening and respond metrics for prometheus.",
		Value: 8080,
	}
	// BootstrapNode tells the beacon node which bootstrap node to connect to
	BootstrapNode = cli.StringFlag{
		Name:  "bootstrap-node",
		Usage: "The address of bootstrap node. Beacon node will connect for peer discovery via DHT",
	}
	// RelayNode tells the beacon node which relay node to connect to.
	RelayNode = cli.StringFlag{
		Name: "relay-node",
		Usage: "The address of relay node. The beacon node will connect to the " +
			"relay node and advertise their address via the relay node to other peers",
	}
	// P2PPort defines the port to be used by libp2p.
	P2PPort = cli.IntFlag{
		Name:  "p2p-port",
		Usage: "The port used by libp2p.",
		Value: 12000,
	}
)
