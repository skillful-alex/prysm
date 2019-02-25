package rpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/shared/bytesutil"

	"github.com/prysmaticlabs/prysm/shared/event"

	ptypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/prysmaticlabs/prysm/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/beacon-chain/internal"
	pbp2p "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/rpc/v1"
	"github.com/prysmaticlabs/prysm/shared/params"
	"github.com/prysmaticlabs/prysm/shared/testutil"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

type faultyPOWChainService struct {
	chainStartFeed *event.Feed
}

func (f *faultyPOWChainService) HasChainStartLogOccurred() (bool, uint64, error) {
	return false, uint64(time.Now().Unix()), nil
}
func (f *faultyPOWChainService) ChainStartFeed() *event.Feed {
	return f.chainStartFeed
}
func (f *faultyPOWChainService) LatestBlockHeight() *big.Int {
	return big.NewInt(0)
}

func (f *faultyPOWChainService) BlockExists(hash common.Hash) (bool, *big.Int, error) {
	return false, big.NewInt(1), errors.New("failed")
}

func (f *faultyPOWChainService) BlockHashByHeight(height *big.Int) (common.Hash, error) {
	return [32]byte{}, errors.New("failed")
}

func (f *faultyPOWChainService) DepositRoot() [32]byte {
	return [32]byte{}
}

type mockPOWChainService struct {
	chainStartFeed    *event.Feed
	latestBlockNumber *big.Int
	hashesByHeight    map[int][]byte
}

func (m *mockPOWChainService) HasChainStartLogOccurred() (bool, uint64, error) {
	return true, uint64(time.Unix(0, 0).Unix()), nil
}
func (m *mockPOWChainService) ChainStartFeed() *event.Feed {
	return m.chainStartFeed
}
func (m *mockPOWChainService) LatestBlockHeight() *big.Int {
	return m.latestBlockNumber
}

func (m *mockPOWChainService) BlockExists(hash common.Hash) (bool, *big.Int, error) {
	// Reverse the map of heights by hash.
	heightsByHash := make(map[[32]byte]int)
	for k, v := range m.hashesByHeight {
		h := bytesutil.ToBytes32(v)
		heightsByHash[h] = k
	}
	val, ok := heightsByHash[hash]
	if !ok {
		return false, nil, fmt.Errorf("could not fetch height for hash: %#x", hash)
	}
	return true, big.NewInt(int64(val)), nil
}

func (m *mockPOWChainService) BlockHashByHeight(height *big.Int) (common.Hash, error) {
	k := int(height.Int64())
	val, ok := m.hashesByHeight[k]
	if !ok {
		return [32]byte{}, fmt.Errorf("could not fetch hash for height: %v", height)
	}
	return bytesutil.ToBytes32(val), nil
}

func (m *mockPOWChainService) DepositRoot() [32]byte {
	root := []byte("depositroot")
	return bytesutil.ToBytes32(root)
}

func TestWaitForChainStart_ContextClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	beaconServer := &BeaconServer{
		ctx: ctx,
		powChainService: &faultyPOWChainService{
			chainStartFeed: new(event.Feed),
		},
		chainService: newMockChainService(),
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := internal.NewMockBeaconService_WaitForChainStartServer(ctrl)
	go func(tt *testing.T) {
		want := "context closed"
		if err := beaconServer.WaitForChainStart(&ptypes.Empty{}, mockStream); !strings.Contains(err.Error(), want) {
			tt.Errorf("Could not call RPC method: %v", err)
		}
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestWaitForChainStart_AlreadyStarted(t *testing.T) {
	beaconServer := &BeaconServer{
		ctx: context.Background(),
		powChainService: &mockPOWChainService{
			chainStartFeed: new(event.Feed),
		},
		chainService: newMockChainService(),
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := internal.NewMockBeaconService_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Send(
		&pb.ChainStartResponse{
			Started:     true,
			GenesisTime: uint64(time.Unix(0, 0).Unix()),
		},
	).Return(nil)
	if err := beaconServer.WaitForChainStart(&ptypes.Empty{}, mockStream); err != nil {
		t.Errorf("Could not call RPC method: %v", err)
	}
}

func TestWaitForChainStart_NotStartedThenLogFired(t *testing.T) {
	hook := logTest.NewGlobal()
	beaconServer := &BeaconServer{
		ctx:            context.Background(),
		chainStartChan: make(chan time.Time, 1),
		powChainService: &faultyPOWChainService{
			chainStartFeed: new(event.Feed),
		},
		chainService: newMockChainService(),
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := internal.NewMockBeaconService_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Send(
		&pb.ChainStartResponse{
			Started:     true,
			GenesisTime: uint64(time.Unix(0, 0).Unix()),
		},
	).Return(nil)
	go func(tt *testing.T) {
		if err := beaconServer.WaitForChainStart(&ptypes.Empty{}, mockStream); err != nil {
			tt.Errorf("Could not call RPC method: %v", err)
		}
		<-exitRoutine
	}(t)
	beaconServer.chainStartChan <- time.Unix(0, 0)
	exitRoutine <- true
	testutil.AssertLogsContain(t, hook, "Sending ChainStart log and genesis time to connected validator clients")
}

func TestLatestAttestation_ContextClosed(t *testing.T) {
	hook := logTest.NewGlobal()
	mockOperationService := &mockOperationService{}
	ctx, cancel := context.WithCancel(context.Background())
	beaconServer := &BeaconServer{
		ctx:              ctx,
		operationService: mockOperationService,
		chainService:     newMockChainService(),
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := internal.NewMockBeaconService_LatestAttestationServer(ctrl)
	go func(tt *testing.T) {
		if err := beaconServer.LatestAttestation(&ptypes.Empty{}, mockStream); err != nil {
			tt.Errorf("Could not call RPC method: %v", err)
		}
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
	testutil.AssertLogsContain(t, hook, "RPC context closed, exiting goroutine")
}

func TestLatestAttestation_FaultyServer(t *testing.T) {
	mockOperationService := &mockOperationService{}
	ctx, cancel := context.WithCancel(context.Background())
	beaconServer := &BeaconServer{
		ctx:                 ctx,
		operationService:    mockOperationService,
		incomingAttestation: make(chan *pbp2p.Attestation, 0),
		chainService:        newMockChainService(),
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	exitRoutine := make(chan bool)
	attestation := &pbp2p.Attestation{}

	mockStream := internal.NewMockBeaconService_LatestAttestationServer(ctrl)
	mockStream.EXPECT().Send(attestation).Return(errors.New("something wrong"))
	// Tests a faulty stream.
	go func(tt *testing.T) {
		if err := beaconServer.LatestAttestation(&ptypes.Empty{}, mockStream); err.Error() != "something wrong" {
			tt.Errorf("Faulty stream should throw correct error, wanted 'something wrong', got %v", err)
		}
		<-exitRoutine
	}(t)

	beaconServer.incomingAttestation <- attestation
	cancel()
	exitRoutine <- true
}

func TestLatestAttestation_SendsCorrectly(t *testing.T) {
	hook := logTest.NewGlobal()
	operationService := &mockOperationService{}
	ctx, cancel := context.WithCancel(context.Background())
	beaconServer := &BeaconServer{
		ctx:                 ctx,
		operationService:    operationService,
		incomingAttestation: make(chan *pbp2p.Attestation, 0),
		chainService:        newMockChainService(),
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	exitRoutine := make(chan bool)
	attestation := &pbp2p.Attestation{}
	mockStream := internal.NewMockBeaconService_LatestAttestationServer(ctrl)
	mockStream.EXPECT().Send(attestation).Return(nil)
	// Tests a good stream.
	go func(tt *testing.T) {
		if err := beaconServer.LatestAttestation(&ptypes.Empty{}, mockStream); err != nil {
			tt.Errorf("Could not call RPC method: %v", err)
		}
		<-exitRoutine
	}(t)
	beaconServer.incomingAttestation <- attestation
	cancel()
	exitRoutine <- true

	testutil.AssertLogsContain(t, hook, "Sending attestation to RPC clients")
}

func TestPendingDeposits_UnknownBlockNum(t *testing.T) {
	p := &mockPOWChainService{
		latestBlockNumber: nil,
	}
	bs := BeaconServer{powChainService: p}

	_, err := bs.PendingDeposits(context.Background(), nil)
	if err.Error() != "latest PoW block number is unknown" {
		t.Errorf("Received unexpected error: %v", err)
	}
}

func TestPendingDeposits_OutsideEth1FollowWindow(t *testing.T) {
	p := &mockPOWChainService{
		latestBlockNumber: big.NewInt(int64(10 + params.BeaconConfig().Eth1FollowDistance)),
	}
	d := &db.BeaconDB{}

	// Using the merkleTreeIndex as the block number for this test...
	readyDeposits := []*pbp2p.Deposit{
		{
			MerkleTreeIndex: 1,
		},
		{
			MerkleTreeIndex: 2,
		},
	}

	recentDeposits := []*pbp2p.Deposit{
		{
			MerkleTreeIndex: params.BeaconConfig().Eth1FollowDistance + 100,
		},
		{
			MerkleTreeIndex: params.BeaconConfig().Eth1FollowDistance + 101,
		},
	}
	ctx := context.Background()
	for _, dp := range append(recentDeposits, readyDeposits...) {
		d.InsertPendingDeposit(ctx, dp, big.NewInt(int64(dp.MerkleTreeIndex)))
	}

	bs := &BeaconServer{
		beaconDB:        d,
		powChainService: p,
		chainService:    newMockChainService(),
	}

	result, err := bs.PendingDeposits(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.PendingDeposits, readyDeposits) {
		t.Errorf("Received unexpected list of deposits: %+v, wanted: %+v", result, readyDeposits)
	}

	// It should also return the recent deposits after their follow window.
	p.latestBlockNumber = big.NewInt(0).Add(p.latestBlockNumber, big.NewInt(10000))
	allResp, err := bs.PendingDeposits(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(allResp.PendingDeposits) != len(recentDeposits)+len(readyDeposits) {
		t.Errorf(
			"Received unexpected number of pending deposits: %d, wanted: %d",
			len(allResp.PendingDeposits),
			len(recentDeposits)+len(readyDeposits),
		)
	}
}

func TestEth1Data_EmptyVotesFetchBlockHashFailure(t *testing.T) {
	db := internal.SetupDB(t)
	defer internal.TeardownDB(t, db)
	beaconServer := &BeaconServer{
		beaconDB:        db,
		powChainService: &faultyPOWChainService{},
	}
	beaconState := &pbp2p.BeaconState{}
	if err := beaconServer.beaconDB.SaveState(beaconState); err != nil {
		t.Fatal(err)
	}
	want := "could not fetch ETH1_FOLLOW_DISTANCE ancestor"
	if _, err := beaconServer.Eth1Data(context.Background(), nil); !strings.Contains(err.Error(), want) {
		t.Errorf("Expected error %v, received %v", want, err)
	}
}

func TestEth1Data_EmptyVotesOk(t *testing.T) {
	db := internal.SetupDB(t)
	defer internal.TeardownDB(t, db)
	powChainService := &mockPOWChainService{
		latestBlockNumber: big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance)),
		hashesByHeight: map[int][]byte{
			0: []byte("hash0"),
		},
	}
	beaconServer := &BeaconServer{
		beaconDB:        db,
		powChainService: powChainService,
	}
	beaconState := &pbp2p.BeaconState{}
	if err := beaconServer.beaconDB.SaveState(beaconState); err != nil {
		t.Fatal(err)
	}
	result, err := beaconServer.Eth1Data(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// If the data vote objects are empty, the deposit root should be the one corresponding
	// to the deposit contract in the powchain service, fetched using powChainService.DepositRoot()
	depositRoot := beaconServer.powChainService.DepositRoot()
	if !bytes.Equal(result.Eth1Data.DepositRootHash32, depositRoot[:]) {
		t.Errorf(
			"Expected deposit roots to match, received %#x == %#x",
			result.Eth1Data.DepositRootHash32,
			depositRoot,
		)
	}
}

func TestEth1Data_NonEmptyVotesSelectsBestVote(t *testing.T) {
	db := internal.SetupDB(t)
	defer internal.TeardownDB(t, db)
	eth1DataVotes := []*pbp2p.Eth1DataVote{
		{
			VoteCount: 1,
			Eth1Data: &pbp2p.Eth1Data{
				BlockHash32:       []byte("block0"),
				DepositRootHash32: []byte("deposit0"),
			},
		},
		{
			VoteCount: 2,
			Eth1Data: &pbp2p.Eth1Data{
				BlockHash32:       []byte("block1"),
				DepositRootHash32: []byte("deposit1"),
			},
		},
		// We include the case in which the vote counts might match and in that
		// case we break ties by checking which block hash has the greatest
		// block height in the eth1.0 chain, accordingly.
		{
			VoteCount: 3,
			Eth1Data: &pbp2p.Eth1Data{
				BlockHash32:       []byte("block2"),
				DepositRootHash32: []byte("deposit2"),
			},
		},
		{
			VoteCount: 3,
			Eth1Data: &pbp2p.Eth1Data{
				BlockHash32:       []byte("block4"),
				DepositRootHash32: []byte("deposit3"),
			},
		},
	}
	beaconState := &pbp2p.BeaconState{
		Eth1DataVotes: eth1DataVotes,
		LatestEth1Data: &pbp2p.Eth1Data{
			BlockHash32: []byte("stub"),
		},
	}
	if err := db.SaveState(beaconState); err != nil {
		t.Fatal(err)
	}
	currentHeight := params.BeaconConfig().Eth1FollowDistance + 5
	beaconServer := &BeaconServer{
		beaconDB: db,
		powChainService: &mockPOWChainService{
			latestBlockNumber: big.NewInt(int64(currentHeight)),
			hashesByHeight: map[int][]byte{
				0: beaconState.LatestEth1Data.BlockHash32,
				1: beaconState.Eth1DataVotes[0].Eth1Data.BlockHash32,
				2: beaconState.Eth1DataVotes[1].Eth1Data.BlockHash32,
				3: beaconState.Eth1DataVotes[3].Eth1Data.BlockHash32,
				// We will give the hash at index 2 in the beacon state's latest eth1 votes
				// priority in being selected as the best vote by giving it the highest block number.
				4: beaconState.Eth1DataVotes[2].Eth1Data.BlockHash32,
			},
		},
	}
	result, err := beaconServer.Eth1Data(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Vote at index 2 should have won the best vote selection mechanism as it had the highest block number
	// despite being tied at vote count with the vote at index 3.
	if !bytes.Equal(result.Eth1Data.BlockHash32, beaconState.Eth1DataVotes[2].Eth1Data.BlockHash32) {
		t.Errorf(
			"Expected block hashes to match, received %#x == %#x",
			result.Eth1Data.BlockHash32,
			beaconState.Eth1DataVotes[2].Eth1Data.BlockHash32,
		)
	}
	if !bytes.Equal(result.Eth1Data.DepositRootHash32, beaconState.Eth1DataVotes[2].Eth1Data.DepositRootHash32) {
		t.Errorf(
			"Expected deposit roots to match, received %#x == %#x",
			result.Eth1Data.DepositRootHash32,
			beaconState.Eth1DataVotes[2].Eth1Data.DepositRootHash32,
		)
	}
}
