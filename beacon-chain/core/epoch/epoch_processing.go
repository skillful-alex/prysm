// Package epoch contains epoch processing libraries. These libraries
// process new balance for the validators, justify and finalize new
// check points, shuffle and reassign validators to different slots and
// shards.
package epoch

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/prysmaticlabs/prysm/shared/hashutil"

	"github.com/prysmaticlabs/prysm/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/validators"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/mathutil"
	"github.com/prysmaticlabs/prysm/shared/params"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

var log = logrus.WithField("prefix", "core/state")

// CanProcessEpoch checks the eligibility to process epoch.
// The epoch can be processed at the end of the last slot of every epoch
//
// Spec pseudocode definition:
//    If (state.slot + 1) % SLOTS_PER_EPOCH == 0:
func CanProcessEpoch(state *pb.BeaconState) bool {
	return (state.Slot+1)%params.BeaconConfig().SlotsPerEpoch == 0
}

// CanProcessEth1Data checks the eligibility to process the eth1 data.
// The eth1 data can be processed every EPOCHS_PER_ETH1_VOTING_PERIOD.
//
// Spec pseudocode definition:
//    If next_epoch % EPOCHS_PER_ETH1_VOTING_PERIOD == 0
func CanProcessEth1Data(state *pb.BeaconState) bool {
	return helpers.NextEpoch(state)%
		params.BeaconConfig().EpochsPerEth1VotingPeriod == 0
}

// CanProcessValidatorRegistry checks the eligibility to process validator registry.
// It checks crosslink committees last changed slot and finalized slot against
// latest change slot.
//
// Spec pseudocode definition:
//    If the following are satisfied:
//		* state.finalized_epoch > state.validator_registry_latest_change_epoch
//		* state.latest_crosslinks[shard].epoch > state.validator_registry_update_epoch
// 			for every shard number shard in [(state.current_epoch_start_shard + i) %
//	 			SHARD_COUNT for i in range(get_current_epoch_committee_count(state) *
//	 			SLOTS_PER_EPOCH)] (that is, for every shard in the current committees)
func CanProcessValidatorRegistry(ctx context.Context, state *pb.BeaconState) bool {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.CanProcessValidatorRegistry")
	defer span.End()

	if state.FinalizedEpoch <= state.ValidatorRegistryUpdateEpoch {
		return false
	}
	shardsProcessed := helpers.CurrentEpochCommitteeCount(state) * params.BeaconConfig().SlotsPerEpoch
	startShard := state.CurrentShufflingStartShard
	for i := startShard; i < shardsProcessed; i++ {

		if state.LatestCrosslinks[i%params.BeaconConfig().ShardCount].Epoch <=
			state.ValidatorRegistryUpdateEpoch {
			return false
		}
	}
	return true
}

// ProcessEth1Data processes eth1 block deposit roots by checking its vote count.
// With sufficient votes (>2*EPOCHS_PER_ETH1_VOTING_PERIOD), it then
// marks the voted Eth1 data as the latest data set.
//
// Official spec definition:
//     if eth1_data_vote.vote_count * 2 > EPOCHS_PER_ETH1_VOTING_PERIOD * SLOTS_PER_EPOCH for
//       some eth1_data_vote in state.eth1_data_votes.
//       (ie. more than half the votes in this voting period were for that value)
//       Set state.latest_eth1_data = eth1_data_vote.eth1_data.
//		 Set state.eth1_data_votes = [].
//
func ProcessEth1Data(ctx context.Context, state *pb.BeaconState) *pb.BeaconState {

	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.ProcessEth1Data")
	defer span.End()

	for _, eth1DataVote := range state.Eth1DataVotes {
		if eth1DataVote.VoteCount*2 > params.BeaconConfig().SlotsPerEpoch*
			params.BeaconConfig().EpochsPerEth1VotingPeriod {
			state.LatestEth1Data = eth1DataVote.Eth1Data
		}
	}
	state.Eth1DataVotes = make([]*pb.Eth1DataVote, 0)
	return state
}

// ProcessJustification processes for justified slot by comparing
// epoch boundary balance and total balance.
//   First, update the justification bitfield:
//     Let new_justified_epoch = state.justified_epoch.
//     Set state.justification_bitfield = state.justification_bitfield << 1.
//     Set state.justification_bitfield |= 2 and new_justified_epoch = previous_epoch if
//       3 * previous_epoch_boundary_attesting_balance >= 2 * previous_total_balance.
//     Set state.justification_bitfield |= 1 and new_justified_epoch = current_epoch if
//       3 * current_epoch_boundary_attesting_balance >= 2 * current_total_balance.
//   Next, update last finalized epoch if possible:
//     Set state.finalized_epoch = state.previous_justified_epoch if (state.justification_bitfield >> 1) % 8
//       == 0b111 and state.previous_justified_epoch == previous_epoch - 2.
//     Set state.finalized_epoch = state.previous_justified_epoch if (state.justification_bitfield >> 1) % 4
//       == 0b11 and state.previous_justified_epoch == previous_epoch - 1.
//     Set state.finalized_epoch = state.justified_epoch if (state.justification_bitfield >> 0) % 8
//       == 0b111 and state.justified_epoch == previous_epoch - 1.
//     Set state.finalized_epoch = state.justified_epoch if (state.justification_bitfield >> 0) % 4
//       == 0b11 and state.justified_epoch == previous_epoch.
//   Finally, update the following:
//     Set state.previous_justified_epoch = state.justified_epoch.
//     Set state.justified_epoch = new_justified_epoch
func ProcessJustification(
	ctx context.Context,
	state *pb.BeaconState,
	thisEpochBoundaryAttestingBalance uint64,
	prevEpochBoundaryAttestingBalance uint64,
	prevTotalBalance uint64,
	totalBalance uint64) *pb.BeaconState {

	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.ProcessJustification")
	defer span.End()

	newJustifiedEpoch := state.JustifiedEpoch
	prevEpoch := helpers.PrevEpoch(state)
	currentEpoch := helpers.CurrentEpoch(state)
	// Shifts all the bits over one to create a new bit for the recent epoch.
	state.JustificationBitfield = state.JustificationBitfield << 1
	log.Infof("Processing Total Balance: %d", totalBalance)
	// If prev prev epoch was justified then we ensure the 2nd bit in the bitfield is set,
	// assign new justified slot to 2 * SLOTS_PER_EPOCH before.
	log.Infof("Previous Epoch Boundary Attesting Balance: %d", prevEpochBoundaryAttestingBalance)
	if 3*prevEpochBoundaryAttestingBalance >= 2*prevTotalBalance {
		state.JustificationBitfield |= 2
		newJustifiedEpoch = prevEpoch
		log.Infof("Previous epoch %d was justified", newJustifiedEpoch-params.BeaconConfig().GenesisEpoch)
	}
	log.Infof("Current Epoch Boundary Attesting Balance: %d", thisEpochBoundaryAttestingBalance)
	// If this epoch was justified then we ensure the 1st bit in the bitfield is set,
	// assign new justified slot to 1 * SLOTS_PER_EPOCH before.
	if 3*thisEpochBoundaryAttestingBalance >= 2*totalBalance {
		state.JustificationBitfield |= 1
		newJustifiedEpoch = currentEpoch
		log.Infof("Current epoch %d was justified", newJustifiedEpoch-params.BeaconConfig().GenesisEpoch)
	}

	// Process finality.
	if state.PreviousJustifiedEpoch == prevEpoch-2 &&
		(state.JustificationBitfield>>1)%8 == 7 {
		state.FinalizedEpoch = state.PreviousJustifiedEpoch
		log.Infof("New Finalized Epoch: %d", state.FinalizedEpoch-params.BeaconConfig().GenesisEpoch)
	}
	if state.PreviousJustifiedEpoch == prevEpoch-1 &&
		(state.JustificationBitfield>>1)%4 == 3 {
		state.FinalizedEpoch = state.PreviousJustifiedEpoch
		log.Infof("New Finalized Epoch: %d", state.FinalizedEpoch-params.BeaconConfig().GenesisEpoch)
	}
	if state.JustifiedEpoch == prevEpoch-1 &&
		(state.JustificationBitfield>>0)%8 == 7 {
		state.FinalizedEpoch = state.JustifiedEpoch
		log.Infof("New Finalized Epoch: %d", state.FinalizedEpoch-params.BeaconConfig().GenesisEpoch)
	}
	if state.JustifiedEpoch == prevEpoch &&
		(state.JustificationBitfield>>0)%4 == 3 {
		state.FinalizedEpoch = state.JustifiedEpoch
		log.Infof("New Finalized Epoch: %d", state.FinalizedEpoch-params.BeaconConfig().GenesisEpoch)
	}
	state.PreviousJustifiedEpoch = state.JustifiedEpoch
	state.JustifiedEpoch = newJustifiedEpoch
	return state
}

// ProcessCrosslinks goes through each crosslink committee and check
// crosslink committee's attested balance * 3 is greater than total balance *2.
// If it's greater then beacon node updates crosslink committee with
// the state epoch and wining root.
//
// Spec pseudocode definition:
//	For every slot in range(get_epoch_start_slot(previous_epoch), get_epoch_start_slot(next_epoch)),
// 	let `crosslink_committees_at_slot = get_crosslink_committees_at_slot(state, slot)`.
// 		For every `(crosslink_committee, shard)` in `crosslink_committees_at_slot`, compute:
// 			Set state.latest_crosslinks[shard] = Crosslink(
// 			epoch=slot_to_epoch(slot), crosslink_data_root=winning_root(crosslink_committee))
// 			if 3 * total_attesting_balance(crosslink_committee) >= 2 * total_balance(crosslink_committee)
func ProcessCrosslinks(
	ctx context.Context,
	state *pb.BeaconState,
	thisEpochAttestations []*pb.PendingAttestation,
	prevEpochAttestations []*pb.PendingAttestation) (*pb.BeaconState, error) {

	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.ProcessCrosslinks")
	defer span.End()

	prevEpoch := helpers.PrevEpoch(state)
	currentEpoch := helpers.CurrentEpoch(state)
	nextEpoch := helpers.NextEpoch(state)
	startSlot := helpers.StartSlot(prevEpoch)
	endSlot := helpers.StartSlot(nextEpoch)

	for i := startSlot; i < endSlot; i++ {
		// RegistryChange is a no-op when requesting slot in current and previous epoch.
		// ProcessCrosslinks will never ask for slot in next epoch.
		crosslinkCommittees, err := helpers.CrosslinkCommitteesAtSlot(state, i, false /* registryChange */)
		if err != nil {
			return nil, fmt.Errorf("could not get committees for slot %d: %v", i-params.BeaconConfig().GenesisSlot, err)
		}
		for _, crosslinkCommittee := range crosslinkCommittees {
			shard := crosslinkCommittee.Shard
			committee := crosslinkCommittee.Committee
			attestingBalance, err := TotalAttestingBalance(ctx, state, shard, thisEpochAttestations, prevEpochAttestations)
			if err != nil {
				return nil, fmt.Errorf("could not get attesting balance for shard committee %d: %v", shard, err)
			}
			totalBalance := TotalBalance(ctx, state, committee)
			if attestingBalance*3 >= totalBalance*2 {
				winningRoot, err := winningRoot(ctx, state, shard, thisEpochAttestations, prevEpochAttestations)
				if err != nil {
					return nil, fmt.Errorf("could not get winning root: %v", err)
				}
				state.LatestCrosslinks[shard] = &pb.Crosslink{
					Epoch:                   currentEpoch,
					CrosslinkDataRootHash32: winningRoot,
				}
			}
		}
	}
	return state, nil
}

// ProcessEjections iterates through every validator and find the ones below
// ejection balance and eject them.
//
// Spec pseudocode definition:
//	def process_ejections(state: BeaconState) -> None:
//    """
//    Iterate through the validator registry
//    and eject active validators with balance below ``EJECTION_BALANCE``.
//    """
//    for index in get_active_validator_indices(state.validator_registry, current_epoch(state)):
//        if state.validator_balances[index] < EJECTION_BALANCE:
//            exit_validator(state, index)
func ProcessEjections(ctx context.Context, state *pb.BeaconState) (*pb.BeaconState, error) {

	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.ProcessEjections")
	defer span.End()

	activeValidatorIndices := helpers.ActiveValidatorIndices(state.ValidatorRegistry, helpers.CurrentEpoch(state))
	for _, index := range activeValidatorIndices {
		if state.ValidatorBalances[index] < params.BeaconConfig().EjectionBalance {
			log.Infof("Validator at index %d EJECTED", index)
			state = validators.ExitValidator(state, index)
		}
	}
	return state, nil
}

// ProcessPrevSlotShardSeed computes and sets current epoch's calculation slot
// and start shard to previous epoch. Then it returns the updated state.
//
// Spec pseudocode definition:
//	Set state.previous_epoch_randao_mix = state.current_epoch_randao_mix
//	Set state.previous_shuffling_start_shard = state.current_shuffling_start_shard
//  Set state.previous_shuffling_seed = state.current_shuffling_seed.
func ProcessPrevSlotShardSeed(state *pb.BeaconState) *pb.BeaconState {
	state.PreviousShufflingEpoch = state.CurrentShufflingEpoch
	state.PreviousShufflingStartShard = state.CurrentShufflingStartShard
	state.PreviousShufflingSeedHash32 = state.CurrentShufflingSeedHash32
	return state
}

// ProcessCurrSlotShardSeed sets the current shuffling information in the beacon state.
//   Set state.current_shuffling_start_shard = (state.current_shuffling_start_shard +
//     get_current_epoch_committee_count(state)) % SHARD_COUNT
//   Set state.current_shuffling_epoch = next_epoch
//   Set state.current_shuffling_seed = generate_seed(state, state.current_shuffling_epoch)
func ProcessCurrSlotShardSeed(state *pb.BeaconState) (*pb.BeaconState, error) {
	state.CurrentShufflingStartShard = (state.CurrentShufflingStartShard +
		helpers.CurrentEpochCommitteeCount(state)) % params.BeaconConfig().ShardCount
	seed, err := helpers.GenerateSeed(state, state.CurrentShufflingEpoch)
	if err != nil {
		return nil, fmt.Errorf("could not update current shuffling seed: %v", err)
	}
	state.CurrentShufflingEpoch = helpers.NextEpoch(state)
	state.CurrentShufflingSeedHash32 = seed[:]
	return state, nil
}

// ProcessPartialValidatorRegistry processes the portion of validator registry
// fields, it doesn't set registry latest change slot. This only gets called if
// validator registry update did not happen.
//
// Spec pseudocode definition:
//	Let epochs_since_last_registry_change = current_epoch -
//		state.validator_registry_update_epoch
//	If epochs_since_last_registry_update > 1 and
//		is_power_of_two(epochs_since_last_registry_update):
// 			set state.current_calculation_epoch = next_epoch
// 			set state.current_shuffling_seed = generate_seed(
// 				state, state.current_calculation_epoch)
func ProcessPartialValidatorRegistry(ctx context.Context, state *pb.BeaconState) (*pb.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.ProcessPartialValidatorRegistry")
	defer span.End()

	epochsSinceLastRegistryChange := helpers.CurrentEpoch(state) -
		state.ValidatorRegistryUpdateEpoch
	if epochsSinceLastRegistryChange > 1 &&
		mathutil.IsPowerOf2(epochsSinceLastRegistryChange) {
		state.CurrentShufflingEpoch = helpers.NextEpoch(state)
		seed, err := helpers.GenerateSeed(state, state.CurrentShufflingEpoch)
		if err != nil {
			return nil, fmt.Errorf("could not generate seed: %v", err)
		}
		state.CurrentShufflingSeedHash32 = seed[:]
	}
	return state, nil
}

// CleanupAttestations removes any attestation in state's latest attestations
// such that the attestation slot is lower than state slot minus epoch length.
// Spec pseudocode definition:
// 		Remove any attestation in state.latest_attestations such
// 		that slot_to_epoch(att.data.slot) < slot_to_epoch(state) - 1
func CleanupAttestations(ctx context.Context, state *pb.BeaconState) *pb.BeaconState {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.CleanupAttestations")
	defer span.End()

	currEpoch := helpers.CurrentEpoch(state)

	var latestAttestations []*pb.PendingAttestation
	for _, attestation := range state.LatestAttestations {
		if helpers.SlotToEpoch(attestation.Data.Slot) >= currEpoch {
			latestAttestations = append(latestAttestations, attestation)
		}
	}
	state.LatestAttestations = latestAttestations
	return state
}

// UpdateLatestActiveIndexRoots updates the latest index roots. Index root
// is computed by hashing validator indices of the next epoch + delay.
//
// Spec pseudocode definition:
// Let e = state.slot // SLOTS_PER_EPOCH.
// Set state.latest_index_roots[(next_epoch + ACTIVATION_EXIT_DELAY) %
// 	LATEST_INDEX_ROOTS_LENGTH] =
// 	hash_tree_root(get_active_validator_indices(state,
// 	next_epoch + ACTIVATION_EXIT_DELAY))
func UpdateLatestActiveIndexRoots(ctx context.Context, state *pb.BeaconState) (*pb.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.UpdateLatestActiveIndexRoots")
	defer span.End()

	nextEpoch := helpers.NextEpoch(state) + params.BeaconConfig().ActivationExitDelay
	validatorIndices := helpers.ActiveValidatorIndices(state.ValidatorRegistry, nextEpoch)
	indicesBytes := []byte{}
	for _, val := range validatorIndices {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, val)
		indicesBytes = append(indicesBytes, buf...)
	}
	indexRoot := hashutil.Hash(indicesBytes)
	state.LatestIndexRootHash32S[nextEpoch%params.BeaconConfig().LatestActiveIndexRootsLength] =
		indexRoot[:]
	return state, nil
}

// UpdateLatestSlashedBalances updates the latest slashed balances. It transfers
// the amount from the current epoch index to next epoch index.
//
// Spec pseudocode definition:
// Set state.latest_slashed_balances[(next_epoch) % LATEST_PENALIZED_EXIT_LENGTH] =
// 	state.latest_slashed_balances[current_epoch % LATEST_PENALIZED_EXIT_LENGTH].
func UpdateLatestSlashedBalances(ctx context.Context, state *pb.BeaconState) *pb.BeaconState {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.UpdateLatestSlashedBalances")
	defer span.End()

	currentEpoch := helpers.CurrentEpoch(state) % params.BeaconConfig().LatestSlashedExitLength
	nextEpoch := helpers.NextEpoch(state) % params.BeaconConfig().LatestSlashedExitLength
	state.LatestSlashedBalances[nextEpoch] = state.LatestSlashedBalances[currentEpoch]
	return state
}

// UpdateLatestRandaoMixes updates the latest seed mixes. It transfers
// the seed mix of current epoch to next epoch.
//
// Spec pseudocode definition:
// Set state.latest_randao_mixes[next_epoch % LATEST_RANDAO_MIXES_LENGTH] =
// 	get_randao_mix(state, current_epoch).
func UpdateLatestRandaoMixes(ctx context.Context, state *pb.BeaconState) (*pb.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.ChainService.state.ProcessEpoch.UpdateLatestRandaoMixes")
	defer span.End()

	nextEpoch := helpers.NextEpoch(state) % params.BeaconConfig().LatestRandaoMixesLength
	randaoMix, err := helpers.RandaoMix(state, helpers.CurrentEpoch(state))
	if err != nil {
		return nil, fmt.Errorf("could not get randaoMix mix: %v", err)
	}

	state.LatestRandaoMixes[nextEpoch] = randaoMix
	return state, nil
}
