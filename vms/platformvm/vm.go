// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"container/heap"
	"errors"
	"fmt"
	"time"

	stdmath "math"

	"github.com/ava-labs/gecko/chains"
	"github.com/ava-labs/gecko/database"
	"github.com/ava-labs/gecko/database/versiondb"
	"github.com/ava-labs/gecko/ids"
	"github.com/ava-labs/gecko/snow"
	"github.com/ava-labs/gecko/snow/consensus/snowman"
	"github.com/ava-labs/gecko/snow/engine/common"
	"github.com/ava-labs/gecko/snow/validators"
	"github.com/ava-labs/gecko/utils/crypto"
	"github.com/ava-labs/gecko/utils/math"
	"github.com/ava-labs/gecko/utils/timer"
	"github.com/ava-labs/gecko/utils/units"
	"github.com/ava-labs/gecko/utils/wrappers"
	"github.com/ava-labs/gecko/vms/components/codec"
	"github.com/ava-labs/gecko/vms/components/core"
)

const (
	// For putting/getting values from state
	accountTypeID uint64 = iota
	validatorsTypeID
	chainsTypeID
	blockTypeID
	subnetsTypeID

	// Delta is the synchrony bound used for safe decision making
	Delta = 10 * time.Second // TODO change to longer period (2 minutes?) before release

	// InflationRate is the maximum inflation rate of AVA from staking
	InflationRate = 1.04

	// BatchSize is the number of decision transaction to place into a block
	BatchSize = 30

	// TODO: Incorporate these constants + turn them into governable parameters

	// MinimumStakeAmount is the minimum amount of $AVA one must bond to be a staker
	MinimumStakeAmount = 10 * units.MicroAva

	// MinimumStakingDuration is the shortest amount of time a staker can bond
	// their funds for.
	MinimumStakingDuration = 24 * time.Hour

	// MaximumStakingDuration is the longest amount of time a staker can bond
	// their funds for.
	MaximumStakingDuration = 365 * 24 * time.Hour

	// NumberOfShares is the number of shares that a delegator is
	// rewarded
	NumberOfShares = 1000000
)

var (
	// taken from https://stackoverflow.com/questions/25065055/what-is-the-maximum-time-time-in-go/32620397#32620397
	maxTime = time.Unix(1<<63-62135596801, 0) // 0 is used because we drop the nano-seconds

	// DefaultSubnetID is the ID of the default subnet
	DefaultSubnetID = ids.Empty

	timestampKey         = ids.NewID([32]byte{'t', 'i', 'm', 'e'})
	currentValidatorsKey = ids.NewID([32]byte{'c', 'u', 'r', 'r', 'e', 'n', 't'})
	pendingValidatorsKey = ids.NewID([32]byte{'p', 'e', 'n', 'd', 'i', 'n', 'g'})
	chainsKey            = ids.NewID([32]byte{'c', 'h', 'a', 'i', 'n', 's'})
	subnetsKey           = ids.NewID([32]byte{'s', 'u', 'b', 'n', 'e', 't', 's'})
)

var (
	errEndOfTime              = errors.New("program time is suspiciously far in the future. Either this codebase was way more successful than expected, or a critical error has occurred")
	errTimeTooAdvanced        = errors.New("this is proposing a time too far in the future")
	errNoPendingBlocks        = errors.New("no pending blocks")
	errUnsupportedFXs         = errors.New("unsupported feature extensions")
	errDB                     = errors.New("problem retrieving/putting value from/in database")
	errDBCurrentValidators    = errors.New("couldn't retrieve current validators from database")
	errDBPutCurrentValidators = errors.New("couldn't put current validators in database")
	errDBPendingValidators    = errors.New("couldn't retrieve pending validators from database")
	errDBPutPendingValidators = errors.New("couldn't put pending validators in database")
	errDBAccount              = errors.New("couldn't retrieve account from database")
	errDBPutAccount           = errors.New("couldn't put account in database")
	errDBChains               = errors.New("couldn't retrieve chain list from database")
	errDBPutChains            = errors.New("couldn't put chain list in database")
	errDBPutBlock             = errors.New("couldn't put block in database")
	errRegisteringType        = errors.New("error registering type with database")
	errMissingBlock           = errors.New("missing block")
)

// Codec does serialization and deserialization
var Codec codec.Codec

func init() {
	Codec = codec.NewDefault()

	errs := wrappers.Errs{}
	errs.Add(
		Codec.RegisterType(&ProposalBlock{}),
		Codec.RegisterType(&Abort{}),
		Codec.RegisterType(&Commit{}),
		Codec.RegisterType(&StandardBlock{}),

		Codec.RegisterType(&UnsignedAddDefaultSubnetValidatorTx{}),
		Codec.RegisterType(&addDefaultSubnetValidatorTx{}),

		Codec.RegisterType(&UnsignedAddNonDefaultSubnetValidatorTx{}),
		Codec.RegisterType(&addNonDefaultSubnetValidatorTx{}),

		Codec.RegisterType(&UnsignedAddDefaultSubnetDelegatorTx{}),
		Codec.RegisterType(&addDefaultSubnetDelegatorTx{}),

		Codec.RegisterType(&UnsignedCreateChainTx{}),
		Codec.RegisterType(&CreateChainTx{}),

		Codec.RegisterType(&UnsignedCreateSubnetTx{}),
		Codec.RegisterType(&CreateSubnetTx{}),

		Codec.RegisterType(&advanceTimeTx{}),
		Codec.RegisterType(&rewardValidatorTx{}),
	)
	if errs.Errored() {
		panic(errs.Err)
	}
}

// VM implements the snowman.ChainVM interface
type VM struct {
	*core.SnowmanVM

	Validators validators.Manager

	// The node's chain manager
	ChainManager chains.Manager

	// Used to create and use keys.
	factory crypto.FactorySECP256K1R

	// Used to get time. Useful for faking time during tests.
	clock timer.Clock

	// Key: block ID
	// Value: the block
	currentBlocks map[[32]byte]Block

	// Transactions that have not been put into blocks yet
	unissuedEvents      *EventHeap
	unissuedDecisionTxs []DecisionTx

	// This timer goes off when it is time for the next validator to add/leave the validator set
	// When it goes off resetTimer() is called, triggering creation of a new block
	timer *timer.Timer
}

// Initialize this blockchain.
// [vm.ChainManager] and [vm.Validators] must be set before this function is called.
func (vm *VM) Initialize(
	ctx *snow.Context,
	db database.Database,
	genesisBytes []byte,
	msgs chan<- common.Message,
	fxs []*common.Fx,
) error {
	ctx.Log.Verbo("initializing platform chain")

	if len(fxs) != 0 {
		return errUnsupportedFXs
	}

	// Initialize the inner VM, which has a lot of boiler-plate logic
	vm.SnowmanVM = &core.SnowmanVM{}
	if err := vm.SnowmanVM.Initialize(ctx, db, vm.unmarshalBlockFunc, msgs); err != nil {
		return err
	}

	// Register this VM's types with the database so we can get/put structs to/from it
	vm.registerDBTypes()

	// If the database is empty, create the platform chain anew using
	// the provided genesis state
	if !vm.DBInitialized() {
		genesis := &Genesis{}
		if err := Codec.Unmarshal(genesisBytes, genesis); err != nil {
			return err
		}
		if err := genesis.Initialize(); err != nil {
			return err
		}

		// Persist accounts that exist at genesis
		for _, account := range genesis.Accounts {
			if err := vm.putAccount(vm.DB, account); err != nil {
				return errDBPutAccount
			}
		}

		// Persist default subnet validator set at genesis
		if err := vm.putCurrentValidators(vm.DB, genesis.Validators, DefaultSubnetID); err != nil {
			return errDBPutCurrentValidators
		}

		// Persist the subnets that exist at genesis (none do)
		if err := vm.putSubnets(vm.DB, []*CreateSubnetTx{}); err != nil {
			return fmt.Errorf("error putting genesis subnets: %v", err)
		}

		// Ensure all chains that the genesis bytes say to create
		// have the right network ID
		filteredChains := []*CreateChainTx{}
		for _, chain := range genesis.Chains {
			if chain.NetworkID == vm.Ctx.NetworkID {
				filteredChains = append(filteredChains, chain)
			} else {
				vm.Ctx.Log.Warn("chain has networkID %d, expected %d", chain.NetworkID, vm.Ctx.NetworkID)
			}
		}

		// Persist the chains that exist at genesis
		if err := vm.putChains(vm.DB, filteredChains); err != nil {
			return errDBPutChains
		}

		// Persist the platform chain's timestamp at genesis
		time := time.Unix(int64(genesis.Timestamp), 0)
		if err := vm.State.PutTime(vm.DB, timestampKey, time); err != nil {
			return errDB
		}

		// There are no pending stakers at genesis
		if err := vm.putPendingValidators(vm.DB, &EventHeap{SortByStartTime: true}, DefaultSubnetID); err != nil {
			return errDBPutPendingValidators
		}

		// Create the genesis block and save it as being accepted
		// (We don't just do genesisBlock.Accept() because then it'd look for genesisBlock's
		// non-existent parent)
		genesisBlock := vm.newCommitBlock(ids.Empty)
		if err := vm.State.PutBlock(vm.DB, genesisBlock); err != nil {
			return errDB
		}
		genesisBlock.onAcceptDB = versiondb.New(vm.DB)
		genesisBlock.CommonBlock.Accept()

		vm.SetDBInitialized()
	}

	// Transactions from clients that have not yet been put into blocks
	// and added to consensus
	vm.unissuedEvents = &EventHeap{SortByStartTime: true}

	vm.currentBlocks = make(map[[32]byte]Block)
	vm.timer = timer.NewTimer(func() {
		vm.Ctx.Lock.Lock()
		defer vm.Ctx.Lock.Unlock()

		vm.resetTimer()
	})
	go ctx.Log.RecoverAndPanic(vm.timer.Dispatch)

	if err := vm.updateValidators(DefaultSubnetID); err != nil {
		ctx.Log.Error("failed to initialize the current validator set: %s", err)
		return err
	}

	// Create all of the chains that the database says exist
	if err := vm.initBlockchains(); err != nil {
		vm.Ctx.Log.Warn("could not retrieve existing chains from database: %s", err)
		return err
	}

	// Build off the most recently accepted block
	vm.SetPreference(vm.LastAccepted())

	return nil
}

// Create all of the chains that the database says should exist
func (vm *VM) initBlockchains() error {
	vm.Ctx.Log.Verbo("platform chain initializing existing blockchains")
	existingChains, err := vm.getChains(vm.DB)
	if err != nil {
		return err
	}
	for _, chain := range existingChains { // Create each blockchain
		chainParams := chains.ChainParameters{
			ID:          chain.ID(),
			GenesisData: chain.GenesisData,
			VMAlias:     chain.VMID.String(),
		}
		for _, fxID := range chain.FxIDs {
			chainParams.FxAliases = append(chainParams.FxAliases, fxID.String())
		}
		vm.ChainManager.CreateChain(chainParams)
	}
	return nil
}

// Shutdown this blockchain
func (vm *VM) Shutdown() {
	vm.timer.Stop()
	if err := vm.DB.Close(); err != nil {
		vm.Ctx.Log.Error("Closing the database failed with %s", err)
	}
}

// BuildBlock builds a block to be added to consensus
func (vm *VM) BuildBlock() (snowman.Block, error) {
	vm.Ctx.Log.Debug("in BuildBlock")
	preferredID := vm.Preferred()

	// If there are pending decision txs, build a block with a batch of them
	if len(vm.unissuedDecisionTxs) > 0 {
		numTxs := BatchSize
		if numTxs > len(vm.unissuedDecisionTxs) {
			numTxs = len(vm.unissuedDecisionTxs)
		}
		var txs []DecisionTx
		txs, vm.unissuedDecisionTxs = vm.unissuedDecisionTxs[:numTxs], vm.unissuedDecisionTxs[numTxs:]
		blk, err := vm.newStandardBlock(preferredID, txs)
		if err != nil {
			return nil, err
		}
		if err := blk.Verify(); err != nil {
			vm.resetTimer()
			return nil, err
		}
		if err := vm.State.PutBlock(vm.DB, blk); err != nil {
			return nil, err
		}
		return blk, vm.DB.Commit()
	}

	// Get the preferred block (which we want to build off)
	preferred, err := vm.getBlock(preferredID)
	vm.Ctx.Log.AssertNoError(err)

	// The database if the preferred block were to be accepted
	var db database.Database
	// The preferred block should always be a decision block
	if preferred, ok := preferred.(decision); ok {
		db = preferred.onAccept()
	} else {
		return nil, errInvalidBlockType
	}

	// The chain time if the preferred block were to be committed
	currentChainTimestamp, err := vm.getTimestamp(db)
	if err != nil {
		return nil, err
	}
	if !currentChainTimestamp.Before(maxTime) {
		return nil, errEndOfTime
	}

	// If the chain time would be the time for the next default subnet validator to leave,
	// then we create a block that removes the validator and proposes they receive a validator reward
	currentValidators, err := vm.getCurrentValidators(db, DefaultSubnetID)
	if err != nil {
		return nil, errDBCurrentValidators
	}
	nextValidatorEndtime := maxTime
	if currentValidators.Len() > 0 {
		nextValidatorEndtime = currentValidators.Peek().EndTime()
	}
	if currentChainTimestamp.Equal(nextValidatorEndtime) {
		stakerTx := currentValidators.Peek()
		rewardValidatorTx, err := vm.newRewardValidatorTx(stakerTx.ID())
		if err != nil {
			return nil, err
		}
		blk, err := vm.newProposalBlock(preferredID, rewardValidatorTx)
		if err != nil {
			return nil, err
		}
		if err := vm.State.PutBlock(vm.DB, blk); err != nil {
			return nil, err
		}
		return blk, vm.DB.Commit()
	}

	// If local time is >= time of the next validator set change,
	// propose moving the chain time forward
	nextValidatorStartTime := vm.nextValidatorChangeTime(db /*start=*/, true)
	nextValidatorEndTime := vm.nextValidatorChangeTime(db /*start=*/, false)

	nextValidatorSetChangeTime := nextValidatorStartTime
	if nextValidatorEndTime.Before(nextValidatorStartTime) {
		nextValidatorSetChangeTime = nextValidatorEndTime
	}

	localTime := vm.clock.Time()
	if !localTime.Before(nextValidatorSetChangeTime) { // time is at or after the time for the next validator to join/leave
		advanceTimeTx, err := vm.newAdvanceTimeTx(nextValidatorSetChangeTime)
		if err != nil {
			return nil, err
		}
		blk, err := vm.newProposalBlock(preferredID, advanceTimeTx)
		if err != nil {
			return nil, err
		}
		if err := vm.State.PutBlock(vm.DB, blk); err != nil {
			return nil, err
		}
		return blk, vm.DB.Commit()
	}

	// Propose adding a new validator but only if their start time is in the
	// future relative to local time (plus Delta)
	syncTime := localTime.Add(Delta)
	for vm.unissuedEvents.Len() > 0 {
		tx := vm.unissuedEvents.Remove()
		if !syncTime.After(tx.StartTime()) {
			blk, err := vm.newProposalBlock(preferredID, tx)
			if err != nil {
				return nil, err
			}
			if err := vm.State.PutBlock(vm.DB, blk); err != nil {
				return nil, err
			}
			return blk, vm.DB.Commit()
		}
		vm.Ctx.Log.Debug("dropping tx to add validator because start time too late")
	}

	vm.Ctx.Log.Debug("BuildBlock returning error (no blocks)")
	return nil, errNoPendingBlocks
}

// ParseBlock implements the snowman.ChainVM interface
func (vm *VM) ParseBlock(bytes []byte) (snowman.Block, error) {
	blockInterface, err := vm.unmarshalBlockFunc(bytes)
	if err != nil {
		return nil, errors.New("problem parsing block")
	}
	block, ok := blockInterface.(snowman.Block)
	if !ok { // in practice should never happen because unmarshalBlockFunc returns a snowman.Block
		return nil, errors.New("problem parsing block")
	}
	// If we have seen this block before, return it with the most up-to-date info
	if block, err := vm.GetBlock(block.ID()); err == nil {
		return block, nil
	}
	vm.State.PutBlock(vm.DB, block)
	vm.DB.Commit()
	return block, nil
}

// GetBlock implements the snowman.ChainVM interface
func (vm *VM) GetBlock(blkID ids.ID) (snowman.Block, error) { return vm.getBlock(blkID) }

func (vm *VM) getBlock(blkID ids.ID) (Block, error) {
	// If block is in memory, return it.
	if blk, exists := vm.currentBlocks[blkID.Key()]; exists {
		return blk, nil
	}
	// Block isn't in memory. If block is in database, return it.
	blkInterface, err := vm.State.GetBlock(vm.DB, blkID)
	if err != nil {
		return nil, err
	}
	if block, ok := blkInterface.(Block); ok {
		return block, nil
	}
	return nil, errors.New("block not found")
}

// SetPreference sets the preferred block to be the one with ID [blkID]
func (vm *VM) SetPreference(blkID ids.ID) {
	if !blkID.Equals(vm.Preferred()) {
		vm.SnowmanVM.SetPreference(blkID)
		vm.resetTimer()
	}
}

// CreateHandlers returns a map where:
// * keys are API endpoint extensions
// * values are API handlers
// See API documentation for more information
func (vm *VM) CreateHandlers() map[string]*common.HTTPHandler {
	// Create a service with name "platform"
	handler := vm.SnowmanVM.NewHandler("platform", &Service{vm: vm})
	return map[string]*common.HTTPHandler{"": handler}
}

// CreateStaticHandlers implements the snowman.ChainVM interface
func (vm *VM) CreateStaticHandlers() map[string]*common.HTTPHandler {
	// Static service's name is platform
	handler := vm.SnowmanVM.NewHandler("platform", &StaticService{})
	return map[string]*common.HTTPHandler{
		"": handler,
	}
}

// Check if there is a block ready to be added to consensus
// If so, notify the consensus engine
func (vm *VM) resetTimer() {
	// If there is a pending CreateChainTx, trigger building of a block
	// with that transaction
	if len(vm.unissuedDecisionTxs) > 0 {
		vm.SnowmanVM.NotifyBlockReady()
		return
	}

	// Get the preferred block
	preferred, err := vm.getBlock(vm.Preferred())
	vm.Ctx.Log.AssertNoError(err)

	// The database if the preferred block were to be committed
	var db database.Database
	// The preferred block should always be a decision block
	if preferred, ok := preferred.(decision); ok {
		db = preferred.onAccept()
	} else {
		vm.Ctx.Log.Error("The preferred block should always be a decision block")
		return
	}

	// The chain time if the preferred block were to be committed
	timestamp, err := vm.getTimestamp(db)
	if err != nil {
		vm.Ctx.Log.Error("could not retrieve timestamp from database")
		return
	}
	if timestamp.Equal(maxTime) {
		vm.Ctx.Log.Error("Program time is suspiciously far in the future. Either this codebase was way more successful than expected, or a critical error has occurred.")
		return
	}

	nextDSValidatorEndTime := vm.nextSubnetValidatorChangeTime(db, DefaultSubnetID, false)
	if timestamp.Equal(nextDSValidatorEndTime) {
		vm.SnowmanVM.NotifyBlockReady() // Should issue a ProposeRewardValidator
		return
	}

	// If local time is >= time of the next change in the validator set,
	// propose moving forward the chain timestamp
	nextValidatorStartTime := vm.nextValidatorChangeTime(db, true)
	nextValidatorEndTime := vm.nextValidatorChangeTime(db, false)

	nextValidatorSetChangeTime := nextValidatorStartTime
	if nextValidatorEndTime.Before(nextValidatorStartTime) {
		nextValidatorSetChangeTime = nextValidatorEndTime
	}

	localTime := vm.clock.Time()
	if !localTime.Before(nextValidatorSetChangeTime) { // time is at or after the time for the next validator to join/leave
		vm.SnowmanVM.NotifyBlockReady() // Should issue a ProposeTimestamp
		return
	}

	syncTime := localTime.Add(Delta)
	for vm.unissuedEvents.Len() > 0 {
		if !syncTime.After(vm.unissuedEvents.Peek().StartTime()) {
			vm.SnowmanVM.NotifyBlockReady() // Should issue a ProposeAddValidator
			return
		}
		// If the tx doesn't meet the syncrony bound, drop it
		vm.unissuedEvents.Remove()
		vm.Ctx.Log.Debug("dropping tx to add validator because its start time has passed")
	}

	waitTime := nextValidatorSetChangeTime.Sub(localTime)
	vm.Ctx.Log.Info("next scheduled event is at %s (%s in the future)", nextValidatorSetChangeTime, waitTime)

	// Wake up when it's time to add/remove the next validator
	vm.timer.SetTimeoutIn(waitTime)
}

// If [start], returns the time at which the next validator (of any subnet) in the pending set starts validating
// Otherwise, returns the time at which the next validator (of any subnet) stops validating
// If no such validator is found, returns maxTime
func (vm *VM) nextValidatorChangeTime(db database.Database, start bool) time.Time {
	earliest := vm.nextSubnetValidatorChangeTime(db, DefaultSubnetID, start)
	subnets, err := vm.getSubnets(db)
	if err != nil {
		return earliest
	}
	for _, subnet := range subnets {
		t := vm.nextSubnetValidatorChangeTime(db, subnet.ID, start)
		if t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

func (vm *VM) nextSubnetValidatorChangeTime(db database.Database, subnetID ids.ID, start bool) time.Time {
	var validators *EventHeap
	var err error
	if start {
		validators, err = vm.getPendingValidators(db, subnetID)
	} else {
		validators, err = vm.getCurrentValidators(db, subnetID)
	}
	if err != nil {
		vm.Ctx.Log.Error("couldn't get validators of subnet with ID %s: %v", subnetID, err)
		return maxTime
	}
	if validators.Len() == 0 {
		vm.Ctx.Log.Verbo("subnet, %s, has no validators", subnetID)
		return maxTime
	}
	return validators.Timestamp()
}

// Returns:
// 1) The validator set of subnet with ID [subnetID] when timestamp is advanced to [timestamp]
// 2) The pending validator set of subnet with ID [subnetID] when timestamp is advanced to [timestamp]
// Note that this method will not remove validators from the current validator set of the default subnet.
// That happens in reward blocks.
func (vm *VM) calculateValidators(db database.Database, timestamp time.Time, subnetID ids.ID) (current, pending *EventHeap, err error) {
	// remove validators whose end time <= [timestamp]
	current, err = vm.getCurrentValidators(db, subnetID)
	if err != nil {
		return nil, nil, err
	}
	if !subnetID.Equals(DefaultSubnetID) { // validators of default subnet removed in rewardValidatorTxs, not here
		for current.Len() > 0 {
			next := current.Peek() // current validator with earliest end time
			if timestamp.Before(next.EndTime()) {
				break
			}
			current.Remove()
		}
	}
	pending, err = vm.getPendingValidators(db, subnetID)
	if err != nil {
		return nil, nil, err
	}
	for pending.Len() > 0 {
		nextTx := pending.Peek() // pending staker with earliest start time
		if timestamp.Before(nextTx.StartTime()) {
			break
		}
		heap.Push(current, nextTx)
		heap.Pop(pending)
	}
	return current, pending, nil
}

func (vm *VM) getValidators(validatorEvents *EventHeap) []validators.Validator {
	vdrMap := make(map[[20]byte]*Validator, validatorEvents.Len())
	for _, event := range validatorEvents.Txs {
		vdr := event.Vdr()
		vdrID := vdr.ID()
		vdrKey := vdrID.Key()
		validator, exists := vdrMap[vdrKey]
		if !exists {
			validator = &Validator{NodeID: vdrID}
			vdrMap[vdrKey] = validator
		}
		weight, err := math.Add64(validator.Wght, vdr.Weight())
		if err != nil {
			weight = stdmath.MaxUint64
		}
		validator.Wght = weight
	}

	vdrList := make([]validators.Validator, len(vdrMap))[:0]
	for _, validator := range vdrMap {
		vdrList = append(vdrList, validator)
	}
	return vdrList
}

func (vm *VM) updateValidators(subnetID ids.ID) error {
	validatorSet, ok := vm.Validators.GetValidatorSet(subnetID)
	if !ok {
		return fmt.Errorf("couldn't get the validator sampler of the %s subnet", subnetID)
	}

	currentValidators, err := vm.getCurrentValidators(vm.DB, subnetID)
	if err != nil {
		return err
	}

	validators := vm.getValidators(currentValidators)
	validatorSet.Set(validators)
	return nil
}
