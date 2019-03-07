package stategenerator

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/prysmaticlabs/prysm/beacon-chain/chaintest/backend"
	"github.com/prysmaticlabs/prysm/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/shared/params"
)

func TestGenerateState_OK(t *testing.T) {
	bd, err := backend.NewSimulatedBackend()
	if err != nil {
		t.Fatalf("Could not create a new simulated backend %v", err)
	}
	privKeys, err := bd.SetupBackend(100)
	if err != nil {
		t.Fatalf("Could not set up backend %v", err)
	}
	beaconDb := bd.DB()
	defer bd.Shutdown()
	defer db.TeardownDB(beaconDb)

	genesisSlot := params.BeaconConfig().GenesisSlot
	slotLimit := uint64(30)

	// Run the simulated chain for 30 slots, to get a state that we can save as finalized.
	for i := uint64(0); i < slotLimit; i++ {
		if err := bd.GenerateBlockAndAdvanceChain(&backend.SimulatedObjects{}, privKeys); err != nil {
			t.Fatalf("Could not generate block and transition state successfully %v for slot %d", err, bd.State().Slot+1)
		}
	}

	if err := beaconDb.SaveFinalizedState(bd.State()); err != nil {
		t.Fatalf("Unable to save finalized state: %v", err)
	}

	// Run the chain for another 30 slots so that we can have this at the current head.
	for i := uint64(0); i < slotLimit; i++ {
		if err := bd.GenerateBlockAndAdvanceChain(&backend.SimulatedObjects{}, privKeys); err != nil {
			t.Fatalf("Could not generate block and transition state successfully %v for slot %d", err, bd.State().Slot+1)
		}
	}

	// Save all in memory blocks
	for _, v := range bd.InMemoryBlocks() {
		if err := beaconDb.SaveBlock(v); err != nil {
			t.Fatalf("Unable to save block %v", err)
		}
		if err := beaconDb.UpdateChainHead(v, bd.State()); err != nil {
			t.Fatalf("Unable to save block %v", err)
		}
	}

	if err := beaconDb.SaveState(bd.State()); err != nil {
		t.Fatalf("Unable to save state in chain %v", err)
	}

	slotToGenerate := genesisSlot + 2*(slotLimit)
	newState, err := GenerateStateFromSlot(context.Background(), beaconDb, slotToGenerate)
	if err != nil {
		t.Fatalf("Unable to generate new state from previous finalized state %v", err)
	}

	if newState.Slot != bd.State().Slot {
		t.Fatalf("The generated state and the current state do not have the same slot, expected: %d but got %d",
			bd.State().Slot, newState.Slot)
	}

	if !proto.Equal(newState, bd.State()) {
		t.Error("Generated and saved states are unequal")
	}
}

func TestGenerateState_WithNilBlocksOK(t *testing.T) {
	bd, err := backend.NewSimulatedBackend()
	if err != nil {
		t.Fatalf("Could not create a new simulated backend %v", err)
	}
	privKeys, err := bd.SetupBackend(100)
	if err != nil {
		t.Fatalf("Could not set up backend %v", err)
	}
	beaconDb := bd.DB()
	defer bd.Shutdown()
	defer db.TeardownDB(beaconDb)

	genesisSlot := params.BeaconConfig().GenesisSlot
	slotLimit := uint64(30)

	// Run the simulated chain for 30 slots, to get a state that we can save as finalized.
	for i := uint64(0); i < slotLimit; i++ {
		if err := bd.GenerateBlockAndAdvanceChain(&backend.SimulatedObjects{}, privKeys); err != nil {
			t.Fatalf("Could not generate block and transition state successfully %v for slot %d", err, bd.State().Slot+1)
		}
	}

	if err := beaconDb.SaveFinalizedState(bd.State()); err != nil {
		t.Fatalf("Unable to save finalized state")
	}

	slotsWithNil := uint64(10)

	// Run the chain for 10 slots with nil blocks.
	for i := uint64(0); i < slotsWithNil; i++ {
		if err := bd.GenerateNilBlockAndAdvanceChain(); err != nil {
			t.Fatalf("Could not generate block and transition state successfully %v for slot %d", err, bd.State().Slot+1)
		}
	}

	for i := uint64(0); i < slotLimit-slotsWithNil; i++ {
		if err := bd.GenerateBlockAndAdvanceChain(&backend.SimulatedObjects{}, privKeys); err != nil {
			t.Fatalf("Could not generate block and transition state successfully %v for slot %d", err, bd.State().Slot+1)
		}
	}

	// Save all in memory blocks.
	for _, v := range bd.InMemoryBlocks() {
		if err := beaconDb.SaveBlock(v); err != nil {
			t.Fatalf("Unable to save block %v", err)
		}
		if err := beaconDb.UpdateChainHead(v, bd.State()); err != nil {
			t.Fatalf("Unable to save block %v", err)
		}
	}

	if err := beaconDb.SaveState(bd.State()); err != nil {
		t.Fatalf("Unable to save state in chain %v", err)
	}

	slotToGenerate := genesisSlot + 2*(slotLimit)
	newState, err := GenerateStateFromSlot(context.Background(), beaconDb, slotToGenerate)
	if err != nil {
		t.Fatalf("Unable to generate new state from previous finalized state %v", err)
	}

	if newState.Slot != bd.State().Slot {
		t.Fatalf("The generated state and the current state do not have the same slot, expected: %d but got %d",
			bd.State().Slot, newState.Slot)
	}

	if !proto.Equal(newState, bd.State()) {
		t.Error("generated and saved states are unequal")
	}
}
