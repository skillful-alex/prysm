package client

import (
	"context"
	"errors"
	"fmt"

	pb "github.com/prysmaticlabs/prysm/proto/beacon/rpc/v1"
	"github.com/prysmaticlabs/prysm/shared/keystore"
	"github.com/prysmaticlabs/prysm/shared/params"
	"github.com/prysmaticlabs/prysm/validator/db"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var log = logrus.WithField("prefix", "validator")

// ValidatorService represents a service to manage the validator client
// routine.
type ValidatorService struct {
	ctx       context.Context
	cancel    context.CancelFunc
	validator Validator
	conn      *grpc.ClientConn
	endpoint  string
	withCert  string
	key       *keystore.Key
	db        *db.ValidatorDB
}

// Config for the validator service.
type Config struct {
	Endpoint     string
	CertFlag     string
	KeystorePath string
	Password     string
	db           *db.ValidatorDB
}

// NewValidatorService creates a new validator service for the service
// registry.
func NewValidatorService(ctx context.Context, cfg *Config) (*ValidatorService, error) {
	ctx, cancel := context.WithCancel(ctx)
	validatorKeyFile := cfg.KeystorePath + params.BeaconConfig().ValidatorPrivkeyFileName
	ks := keystore.NewKeystore(cfg.KeystorePath)
	key, err := ks.GetKey(validatorKeyFile, cfg.Password)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("could not get private key: %v", err)
	}
	return &ValidatorService{
		ctx:      ctx,
		cancel:   cancel,
		endpoint: cfg.Endpoint,
		withCert: cfg.CertFlag,
		key:      key,
		db:       cfg.db,
	}, nil
}

// Start the validator service. Launches the main go routine for the validator
// client.
func (v *ValidatorService) Start() {
	log.WithField("publicKey", fmt.Sprintf("%#x", v.key.PublicKey.Marshal())).Info("Initializing new validator service")

	var dialOpt grpc.DialOption
	if v.withCert != "" {
		creds, err := credentials.NewClientTLSFromFile(v.withCert, "")
		if err != nil {
			log.Errorf("Could not get valid credentials: %v", err)
			return
		}
		dialOpt = grpc.WithTransportCredentials(creds)
	} else {
		dialOpt = grpc.WithInsecure()
		log.Warn("You are using an insecure gRPC connection! Please provide a certificate and key to use a secure connection.")
	}
	conn, err := grpc.DialContext(v.ctx, v.endpoint, dialOpt, grpc.WithStatsHandler(&ocgrpc.ClientHandler{}))
	if err != nil {
		log.Errorf("Could not dial endpoint: %s, %v", v.endpoint, err)
		return
	}
	log.Info("Successfully started gRPC connection")
	v.conn = conn
	v.validator = &validator{
		beaconClient:    pb.NewBeaconServiceClient(v.conn),
		validatorClient: pb.NewValidatorServiceClient(v.conn),
		attesterClient:  pb.NewAttesterServiceClient(v.conn),
		proposerClient:  pb.NewProposerServiceClient(v.conn),
		key:             v.key,
		db:              v.db,
	}
	go run(v.ctx, v.validator)
}

// Stop the validator service.
func (v *ValidatorService) Stop() error {
	v.cancel()
	log.Info("Stopping service")
	if v.conn != nil {
		return v.conn.Close()
	}
	return nil
}

// Status ...
//
// WIP - not done.
func (v *ValidatorService) Status() error {
	if v.conn == nil {
		return errors.New("no connection to beacon RPC")
	}
	return nil
}
