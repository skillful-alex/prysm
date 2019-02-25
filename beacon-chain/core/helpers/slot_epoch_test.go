package helpers

import (
	"testing"

	pb "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/params"
)

func TestSlotToEpoch(t *testing.T) {
	tests := []struct {
		slot  uint64
		epoch uint64
	}{
		{slot: 0, epoch: 0 / params.BeaconConfig().EpochLength},
		{slot: 50, epoch: 0 / params.BeaconConfig().EpochLength},
		{slot: 64, epoch: 64 / params.BeaconConfig().EpochLength},
		{slot: 128, epoch: 128 / params.BeaconConfig().EpochLength},
		{slot: 200, epoch: 200 / params.BeaconConfig().EpochLength},
	}
	for _, tt := range tests {
		if tt.epoch != SlotToEpoch(tt.slot) {
			t.Errorf("SlotToEpoch(%d) = %d, wanted: %d", tt.slot, SlotToEpoch(tt.slot), tt.epoch)
		}
	}
}

func TestCurrentEpoch(t *testing.T) {
	tests := []struct {
		slot  uint64
		epoch uint64
	}{
		{slot: 0, epoch: 0 / params.BeaconConfig().EpochLength},
		{slot: 50, epoch: 0 / params.BeaconConfig().EpochLength},
		{slot: 64, epoch: 64 / params.BeaconConfig().EpochLength},
		{slot: 128, epoch: 128 / params.BeaconConfig().EpochLength},
		{slot: 200, epoch: 200 / params.BeaconConfig().EpochLength},
	}
	for _, tt := range tests {
		state := &pb.BeaconState{Slot: tt.slot}
		if tt.epoch != CurrentEpoch(state) {
			t.Errorf("CurrentEpoch(%d) = %d, wanted: %d", state.Slot, CurrentEpoch(state), tt.epoch)
		}
	}
}

func TestPrevEpoch(t *testing.T) {
	tests := []struct {
		slot  uint64
		epoch uint64
	}{
		{slot: 0, epoch: 0 / params.BeaconConfig().EpochLength},
		{slot: 50, epoch: 0 / params.BeaconConfig().EpochLength},
		{slot: 64, epoch: 64/params.BeaconConfig().EpochLength - 1},
		{slot: 128, epoch: 128/params.BeaconConfig().EpochLength - 1},
		{slot: 200, epoch: 200/params.BeaconConfig().EpochLength - 1},
	}
	for _, tt := range tests {
		state := &pb.BeaconState{Slot: tt.slot}
		if tt.epoch != PrevEpoch(state) {
			t.Errorf("PrevEpoch(%d) = %d, wanted: %d", state.Slot, PrevEpoch(state), tt.epoch)
		}
	}
}

func TestNextEpoch(t *testing.T) {
	tests := []struct {
		slot  uint64
		epoch uint64
	}{
		{slot: 0, epoch: 0/params.BeaconConfig().EpochLength + 1},
		{slot: 50, epoch: 0/params.BeaconConfig().EpochLength + 1},
		{slot: 64, epoch: 64/params.BeaconConfig().EpochLength + 1},
		{slot: 128, epoch: 128/params.BeaconConfig().EpochLength + 1},
		{slot: 200, epoch: 200/params.BeaconConfig().EpochLength + 1},
	}
	for _, tt := range tests {
		state := &pb.BeaconState{Slot: tt.slot}
		if tt.epoch != NextEpoch(state) {
			t.Errorf("NextEpoch(%d) = %d, wanted: %d", state.Slot, NextEpoch(state), tt.epoch)
		}
	}
}

func TestEpochStartSlot(t *testing.T) {
	tests := []struct {
		epoch     uint64
		startSlot uint64
	}{
		{epoch: 0, startSlot: 0 * params.BeaconConfig().EpochLength},
		{epoch: 1, startSlot: 1 * params.BeaconConfig().EpochLength},
		{epoch: 10, startSlot: 10 * params.BeaconConfig().EpochLength},
	}
	for _, tt := range tests {
		state := &pb.BeaconState{Slot: tt.epoch}
		if tt.startSlot != StartSlot(tt.epoch) {
			t.Errorf("StartSlot(%d) = %d, wanted: %d", state.Slot, StartSlot(tt.epoch), tt.startSlot)
		}
	}
}

func TestAttestationCurrentEpoch(t *testing.T) {
	tests := []struct {
		slot  uint64
		epoch uint64
	}{
		{slot: 0 * params.BeaconConfig().EpochLength, epoch: 0},
		{slot: 1 * params.BeaconConfig().EpochLength, epoch: 1},
		{slot: 10 * params.BeaconConfig().EpochLength, epoch: 10},
	}
	for _, tt := range tests {
		attData := &pb.AttestationData{Slot: tt.slot}
		if tt.epoch != AttestationCurrentEpoch(attData) {
			t.Errorf("AttestationEpoch(%d) = %d, wanted: %d", attData.Slot, AttestationCurrentEpoch(attData), tt.epoch)
		}
	}
}
