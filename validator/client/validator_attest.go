package client

import (
	"context"
	"fmt"
	"time"

	pbp2p "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/rpc/v1"
	"github.com/prysmaticlabs/prysm/shared/params"
	"go.opencensus.io/trace"
)

var delay = params.BeaconConfig().SecondsPerSlot / 2

// AttestToBlockHead completes the validator client's attester responsibility at a given slot.
// It fetches the latest beacon block head along with the latest canonical beacon state
// information in order to sign the block and include information about the validator's
// participation in voting on the block.
func (v *validator) AttestToBlockHead(ctx context.Context, slot uint64) {
	ctx, span := trace.StartSpan(ctx, "validator.AttestToBlockHead")
	defer span.End()
	log.Info("Attesting...")
	// First the validator should construct attestation_data, an AttestationData
	// object based upon the state at the assigned slot.
	attData := &pbp2p.AttestationData{
		Slot:                    slot,
		CrosslinkDataRootHash32: params.BeaconConfig().ZeroHash[:], // Stub for Phase 0.
	}
	// We fetch the validator index as it is necessary to generate the aggregation
	// bitfield of the attestation itself.
	pubKey := v.key.PublicKey.Marshal()
	idxReq := &pb.ValidatorIndexRequest{
		PublicKey: pubKey,
	}
	validatorIndexRes, err := v.validatorClient.ValidatorIndex(ctx, idxReq)
	if err != nil {
		log.Errorf("Could not fetch validator index: %v", err)
		return
	}
	req := &pb.ValidatorEpochAssignmentsRequest{
		EpochStart: slot,
		PublicKey:  pubKey,
	}
	resp, err := v.validatorClient.CommitteeAssignment(ctx, req)
	if err != nil {
		log.Errorf("Could not fetch crosslink committees at slot %d: %v",
			slot-params.BeaconConfig().GenesisSlot, err)
		return
	}
	// Set the attestation data's shard as the shard associated with the validator's
	// committee as retrieved by CrosslinkCommitteesAtSlot.
	attData.Shard = resp.Shard

	// Fetch other necessary information from the beacon node in order to attest
	// including the justified epoch, epoch boundary information, and more.
	infoReq := &pb.AttestationDataRequest{
		Slot:  slot,
		Shard: resp.Shard,
	}
	infoRes, err := v.attesterClient.AttestationDataAtSlot(ctx, infoReq)
	if err != nil {
		log.Errorf("Could not fetch necessary info to produce attestation at slot %d: %v",
			slot-params.BeaconConfig().GenesisSlot, err)
		return
	}
	log.Infof("Attestation info response: %v", infoRes)
	// Set the attestation data's beacon block root = hash_tree_root(head) where head
	// is the validator's view of the head block of the beacon chain during the slot.
	attData.BeaconBlockRootHash32 = infoRes.BeaconBlockRootHash32
	// Set the attestation data's epoch boundary root = hash_tree_root(epoch_boundary)
	// where epoch_boundary is the block at the most recent epoch boundary in the
	// chain defined by head -- i.e. the BeaconBlock where block.slot == get_epoch_start_slot(slot_to_epoch(head.slot)).
	attData.EpochBoundaryRootHash32 = infoRes.EpochBoundaryRootHash32
	// Set the attestation data's latest crosslink root = state.latest_crosslinks[shard].shard_block_root
	// where state is the beacon state at head and shard is the validator's assigned shard.
	attData.LatestCrosslink = infoRes.LatestCrosslink
	// Set the attestation data's justified epoch = state.justified_epoch where state
	// is the beacon state at the head.
	attData.JustifiedEpoch = infoRes.JustifiedEpoch
	// Set the attestation data's justified block root = hash_tree_root(justified_block) where
	// justified_block is the block at state.justified_epoch in the chain defined by head.
	// On the server side, this is fetched by calling get_block_root(state, justified_epoch).
	attData.JustifiedBlockRootHash32 = infoRes.JustifiedBlockRootHash32

	// The validator now creates an Attestation object using the AttestationData as
	// set in the code above after all properties have been set.
	attestation := &pbp2p.Attestation{
		Data: attData,
	}

	// We set the custody bitfield to an slice of zero values as a stub for phase 0
	// of length len(committee)+7 // 8.
	attestation.CustodyBitfield = make([]byte, (len(resp.Committee)+7)/8)

	// We set the attestation's aggregation bitfield by determining the index in the committee
	// corresponding to the validator and modifying the bitfield itself.
	aggregationBitfield := make([]byte, (len(resp.Committee)+7)/8)
	var indexIntoCommittee uint
	for i, validator := range resp.Committee {
		if validator == validatorIndexRes.Index {
			indexIntoCommittee = uint(i)
			break
		}
	}
	if len(aggregationBitfield) == 0 {
		log.Error("Aggregation bitfield is empty so unable to attest to block head")
		return
	}
	aggregationBitfield[indexIntoCommittee/8] |= 1 << (indexIntoCommittee % 8)
	// Note: calling get_attestation_participants(state, attestation.data, attestation.aggregation_bitfield)
	// should return a list of length equal to 1, containing validator_index.
	attestation.AggregationBitfield = aggregationBitfield

	// TODO(#1366): Use BLS to generate an aggregate signature.
	attestation.AggregateSignature = []byte("signed")

	duration := time.Duration(slot*params.BeaconConfig().SecondsPerSlot+delay) * time.Second
	timeToBroadcast := time.Unix(int64(v.genesisTime), 0).Add(duration)
	_, sleepSpan := trace.StartSpan(ctx, "validator.AttestToBlockHead_sleepUntilTimeToBroadcast")
	time.Sleep(time.Until(timeToBroadcast))
	sleepSpan.End()
	log.Infof("Produced attestation: %v", attestation)
	attestRes, err := v.attesterClient.AttestHead(ctx, attestation)
	if err != nil {
		log.Errorf("Could not submit attestation to beacon node: %v", err)
		return
	}
	log.WithField(
		"hash", fmt.Sprintf("%#x", attestRes.AttestationHash),
	).Infof("Submitted attestation successfully with hash %#x", attestRes.AttestationHash)
}
