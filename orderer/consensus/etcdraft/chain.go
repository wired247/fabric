/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"context"
	"encoding/pem"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
)

const (
	// DefaultSnapshotCatchUpEntries is the default number of entries
	// to preserve in memory when a snapshot is taken. This is for
	// slow followers to catch up.
	DefaultSnapshotCatchUpEntries = uint64(20)

	// DefaultSnapshotInterval is the default snapshot interval. It is
	// used if SnapshotInterval is not provided in channel config options.
	// It is needed to enforce snapshot being set.
	DefaultSnapshotInterval = 100 * MEGABYTE // 100MB
)

//go:generate mockery -dir . -name Configurator -case underscore -output ./mocks/

// Configurator is used to configure the communication layer
// when the chain starts.
type Configurator interface {
	Configure(channel string, newNodes []cluster.RemoteNode)
}

//go:generate counterfeiter -o mocks/mock_rpc.go . RPC

// RPC is used to mock the transport layer in tests.
type RPC interface {
	SendConsensus(dest uint64, msg *orderer.ConsensusRequest) error
	SendSubmit(dest uint64, request *orderer.SubmitRequest) error
}

//go:generate counterfeiter -o mocks/mock_blockpuller.go . BlockPuller

// BlockPuller is used to pull blocks from other OSN
type BlockPuller interface {
	PullBlock(seq uint64) *common.Block
	Close()
}

// CreateBlockPuller is a function to create BlockPuller on demand.
// It is passed into chain initializer so that tests could mock this.
type CreateBlockPuller func() (BlockPuller, error)

// Options contains all the configurations relevant to the chain.
type Options struct {
	RaftID uint64

	Clock clock.Clock

	WALDir       string
	SnapDir      string
	SnapInterval uint32

	// This is configurable mainly for testing purpose. Users are not
	// expected to alter this. Instead, DefaultSnapshotCatchUpEntries is used.
	SnapshotCatchUpEntries uint64

	MemoryStorage MemoryStorage
	Logger        *flogging.FabricLogger

	TickInterval    time.Duration
	ElectionTick    int
	HeartbeatTick   int
	MaxSizePerMsg   uint64
	MaxInflightMsgs int

	RaftMetadata *etcdraft.RaftMetadata
}

type submit struct {
	req    *orderer.SubmitRequest
	leader chan uint64
}

type gc struct {
	index uint64
	state raftpb.ConfState
	data  []byte
}

// Chain implements consensus.Chain interface.
type Chain struct {
	configurator Configurator

	rpc RPC

	raftID    uint64
	channelID string

	submitC  chan *submit
	applyC   chan apply
	observeC chan<- raft.SoftState // Notifies external observer on leader change (passed in optionally as an argument for tests)
	haltC    chan struct{}         // Signals to goroutines that the chain is halting
	doneC    chan struct{}         // Closes when the chain halts
	startC   chan struct{}         // Closes when the node is started
	snapC    chan *raftpb.Snapshot // Signal to catch up with snapshot
	gcC      chan *gc              // Signal to take snapshot

	errorCLock sync.RWMutex
	errorC     chan struct{} // returned by Errored()

	raftMetadataLock     sync.RWMutex
	confChangeInProgress *raftpb.ConfChange
	justElected          bool // this is true when node has just been elected
	configInflight       bool // this is true when there is config block or ConfChange in flight
	blockInflight        int  // number of in flight blocks

	clock clock.Clock // Tests can inject a fake clock

	support consensus.ConsenterSupport

	appliedIndex uint64

	// needed by snapshotting
	sizeLimit        uint32 // SnapshotInterval in bytes
	accDataSize      uint32 // accumulative data size since last snapshot
	lastSnapBlockNum uint64
	confState        raftpb.ConfState // Etcdraft requires ConfState to be persisted within snapshot

	createPuller CreateBlockPuller // func used to create BlockPuller on demand

	fresh bool // indicate if this is a fresh raft node

	node *node
	opts Options

	logger *flogging.FabricLogger
}

// NewChain constructs a chain object.
func NewChain(
	support consensus.ConsenterSupport,
	opts Options,
	conf Configurator,
	rpc RPC,
	f CreateBlockPuller,
	observeC chan<- raft.SoftState) (*Chain, error) {

	lg := opts.Logger.With("channel", support.ChainID(), "node", opts.RaftID)

	fresh := !wal.Exist(opts.WALDir)
	storage, err := CreateStorage(lg, opts.WALDir, opts.SnapDir, opts.MemoryStorage)
	if err != nil {
		return nil, errors.Errorf("failed to restore persisted raft data: %s", err)
	}

	if opts.SnapshotCatchUpEntries == 0 {
		storage.SnapshotCatchUpEntries = DefaultSnapshotCatchUpEntries
	} else {
		storage.SnapshotCatchUpEntries = opts.SnapshotCatchUpEntries
	}

	sizeLimit := opts.SnapInterval
	if sizeLimit == 0 {
		sizeLimit = DefaultSnapshotInterval
	}

	// get block number in last snapshot, if exists
	var snapBlkNum uint64
	if s := storage.Snapshot(); !raft.IsEmptySnap(s) {
		b := utils.UnmarshalBlockOrPanic(s.Data)
		snapBlkNum = b.Header.Number
	}

	c := &Chain{
		configurator:     conf,
		rpc:              rpc,
		channelID:        support.ChainID(),
		raftID:           opts.RaftID,
		submitC:          make(chan *submit),
		applyC:           make(chan apply),
		haltC:            make(chan struct{}),
		doneC:            make(chan struct{}),
		startC:           make(chan struct{}),
		snapC:            make(chan *raftpb.Snapshot),
		errorC:           make(chan struct{}),
		gcC:              make(chan *gc),
		observeC:         observeC,
		support:          support,
		fresh:            fresh,
		appliedIndex:     opts.RaftMetadata.RaftIndex,
		sizeLimit:        sizeLimit,
		lastSnapBlockNum: snapBlkNum,
		createPuller:     f,
		clock:            opts.Clock,
		logger:           lg,
		opts:             opts,
	}

	// DO NOT use Applied option in config, see https://github.com/etcd-io/etcd/issues/10217
	// We guard against replay of written blocks in `entriesToApply` instead.
	config := &raft.Config{
		ID:              c.raftID,
		ElectionTick:    c.opts.ElectionTick,
		HeartbeatTick:   c.opts.HeartbeatTick,
		MaxSizePerMsg:   c.opts.MaxSizePerMsg,
		MaxInflightMsgs: c.opts.MaxInflightMsgs,
		Logger:          c.logger,
		Storage:         c.opts.MemoryStorage,
		// PreVote prevents reconnected node from disturbing network.
		// See etcd/raft doc for more details.
		PreVote:                   true,
		CheckQuorum:               true,
		DisableProposalForwarding: true, // This prevents blocks from being accidentally proposed by followers
	}

	c.node = &node{
		chainID:      c.channelID,
		chain:        c,
		logger:       c.logger,
		storage:      storage,
		rpc:          c.rpc,
		config:       config,
		tickInterval: c.opts.TickInterval,
		clock:        c.clock,
		metadata:     c.opts.RaftMetadata,
	}

	return c, nil
}

// Start instructs the orderer to begin serving the chain and keep it current.
func (c *Chain) Start() {
	c.logger.Infof("Starting Raft node")

	if err := c.configureComm(); err != nil {
		c.logger.Errorf("Failed to start chain, aborting: +%v", err)
		close(c.doneC)
		return
	}

	c.node.start(c.fresh, c.support.Height() > 1)
	close(c.startC)
	close(c.errorC)

	go c.gc()
	go c.serveRequest()
}

// Order submits normal type transactions for ordering.
func (c *Chain) Order(env *common.Envelope, configSeq uint64) error {
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, 0)
}

// Configure submits config type transactions for ordering.
func (c *Chain) Configure(env *common.Envelope, configSeq uint64) error {
	if err := c.checkConfigUpdateValidity(env); err != nil {
		return err
	}
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, 0)
}

// Validate the config update for being of Type A or Type B as described in the design doc.
func (c *Chain) checkConfigUpdateValidity(ctx *common.Envelope) error {
	var err error
	payload, err := utils.UnmarshalPayload(ctx.Payload)
	if err != nil {
		return err
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return err
	}

	switch chdr.Type {
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		return nil
	case int32(common.HeaderType_CONFIG):
		configUpdate, err := configtx.UnmarshalConfigUpdateFromPayload(payload)
		if err != nil {
			return err
		}

		// Check that only the ConsensusType is updated in the write-set
		if ordererConfigGroup, ok := configUpdate.WriteSet.Groups["Orderer"]; ok {
			if val, ok := ordererConfigGroup.Values["ConsensusType"]; ok {
				return c.checkConsentersSet(val)
			}
		}
		return nil

	default:
		return errors.Errorf("config transaction has unknown header type")
	}
}

// WaitReady blocks when the chain:
// - is catching up with other nodes using snapshot
//
// In any other case, it returns right away.
func (c *Chain) WaitReady() error {
	if err := c.isRunning(); err != nil {
		return err
	}

	select {
	case c.submitC <- nil:
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	}

	return nil
}

// Errored returns a channel that closes when the chain stops.
func (c *Chain) Errored() <-chan struct{} {
	c.errorCLock.RLock()
	defer c.errorCLock.RUnlock()
	return c.errorC
}

// Halt stops the chain.
func (c *Chain) Halt() {
	select {
	case <-c.startC:
	default:
		c.logger.Warnf("Attempted to halt a chain that has not started")
		return
	}

	select {
	case c.haltC <- struct{}{}:
	case <-c.doneC:
		return
	}
	<-c.doneC
}

func (c *Chain) isRunning() error {
	select {
	case <-c.startC:
	default:
		return errors.Errorf("chain is not started")
	}

	select {
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	default:
	}

	return nil
}

// Consensus passes the given ConsensusRequest message to the raft.Node instance
func (c *Chain) Consensus(req *orderer.ConsensusRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		return err
	}

	stepMsg := &raftpb.Message{}
	if err := proto.Unmarshal(req.Payload, stepMsg); err != nil {
		return fmt.Errorf("failed to unmarshal StepRequest payload to Raft Message: %s", err)
	}

	if err := c.node.Step(context.TODO(), *stepMsg); err != nil {
		return fmt.Errorf("failed to process Raft Step message: %s", err)
	}

	return nil
}

// Submit forwards the incoming request to:
// - the local serveRequest goroutine if this is leader
// - the actual leader via the transport mechanism
// The call fails if there's no leader elected yet.
func (c *Chain) Submit(req *orderer.SubmitRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		return err
	}

	leadC := make(chan uint64, 1)
	select {
	case c.submitC <- &submit{req, leadC}:
		lead := <-leadC
		if lead == raft.None {
			return errors.Errorf("no Raft leader")
		}

		if lead != c.raftID {
			return c.rpc.SendSubmit(lead, req)
		}

	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	}

	return nil
}

type apply struct {
	entries []raftpb.Entry
	soft    *raft.SoftState
}

func isCandidate(state raft.StateType) bool {
	return state == raft.StatePreCandidate || state == raft.StateCandidate
}

func (c *Chain) serveRequest() {
	ticking := false
	timer := c.clock.NewTimer(time.Second)
	// we need a stopped timer rather than nil,
	// because we will be select waiting on timer.C()
	if !timer.Stop() {
		<-timer.C()
	}

	// if timer is already started, this is a no-op
	start := func() {
		if !ticking {
			ticking = true
			timer.Reset(c.support.SharedConfig().BatchTimeout())
		}
	}

	stop := func() {
		if !timer.Stop() && ticking {
			// we only need to drain the channel if the timer expired (not explicitly stopped)
			<-timer.C()
		}
		ticking = false
	}

	var soft raft.SoftState
	submitC := c.submitC
	var bc *blockCreator

	becomeLeader := func() {
		c.blockInflight = 0
		c.justElected = true
		submitC = nil

		// if there is unfinished ConfChange, we should resume the effort to propose it as
		// new leader, and wait for it to be committed before start serving new requests.
		if cc := c.getInFlightConfChange(); cc != nil {
			if err := c.node.ProposeConfChange(context.TODO(), *cc); err != nil {
				c.logger.Warnf("Failed to propose configuration update to Raft node: %s", err)
			}

			c.confChangeInProgress = cc
			c.configInflight = true
		}
	}

	becomeFollower := func() {
		c.blockInflight = 0
		_ = c.support.BlockCutter().Cut()
		stop()
		submitC = c.submitC
		bc = nil
	}

	for {
		select {
		case s := <-submitC:
			if s == nil {
				// polled by `WaitReady`
				continue
			}

			if soft.RaftState == raft.StatePreCandidate || soft.RaftState == raft.StateCandidate {
				s.leader <- raft.None
				continue
			}

			s.leader <- soft.Lead
			if soft.Lead != c.raftID {
				continue
			}

			batches, pending, err := c.ordered(s.req)
			if err != nil {
				c.logger.Errorf("Failed to order message: %s", err)
			}
			if pending {
				start() // no-op if timer is already started
			} else {
				stop()
			}

			c.propose(bc, batches...)

			if c.configInflight {
				c.logger.Info("Received config block, pause accepting transaction till it is committed")
				submitC = nil
			} else if c.blockInflight >= c.opts.MaxInflightMsgs {
				c.logger.Debugf("Number of in-flight blocks (%d) reaches limit (%d), pause accepting transaction",
					c.blockInflight, c.opts.MaxInflightMsgs)
				submitC = nil
			}

		case app := <-c.applyC:
			if app.soft != nil {
				newLeader := atomic.LoadUint64(&app.soft.Lead) // etcdraft requires atomic access
				if newLeader != soft.Lead {
					c.logger.Infof("Raft leader changed: %d -> %d", soft.Lead, newLeader)

					if newLeader == c.raftID {
						becomeLeader()
					}

					if soft.Lead == c.raftID {
						becomeFollower()
					}
				}

				foundLeader := soft.Lead == raft.None && newLeader != raft.None
				quitCandidate := isCandidate(soft.RaftState) && !isCandidate(app.soft.RaftState)

				if foundLeader || quitCandidate {
					c.errorCLock.Lock()
					c.errorC = make(chan struct{})
					c.errorCLock.Unlock()
				}

				if isCandidate(app.soft.RaftState) || newLeader == raft.None {
					select {
					case <-c.errorC:
					default:
						close(c.errorC)
					}
				}

				soft = raft.SoftState{Lead: newLeader, RaftState: app.soft.RaftState}

				// notify external observer
				select {
				case c.observeC <- soft:
				default:
				}
			}

			c.apply(app.entries)

			if c.justElected {
				msgInflight := c.node.lastIndex() > c.appliedIndex
				if msgInflight || c.configInflight {
					c.logger.Debugf("There are in flight blocks, new leader should not serve requests")
					continue
				}

				c.logger.Infof("Start accepting requests as Raft leader")
				lastBlock := c.support.Block(c.support.Height() - 1)
				bc = &blockCreator{
					hash:   lastBlock.Header.Hash(),
					number: lastBlock.Header.Number,
					logger: c.logger,
				}
				submitC = c.submitC
				c.justElected = false
			} else if c.configInflight {
				c.logger.Info("Config block or ConfChange in flight, pause accepting transaction")
				submitC = nil
			} else if c.blockInflight < c.opts.MaxInflightMsgs {
				submitC = c.submitC
			}

		case <-timer.C():
			ticking = false

			batch := c.support.BlockCutter().Cut()
			if len(batch) == 0 {
				c.logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}

			c.logger.Debugf("Batch timer expired, creating block")
			c.propose(bc, batch) // we are certain this is normal block, no need to block

		case sn := <-c.snapC:
			if sn.Metadata.Index <= c.appliedIndex {
				c.logger.Debugf("Skip snapshot taken at index %d, because it is behind current applied index %d", sn.Metadata.Index, c.appliedIndex)
				break
			}

			b := utils.UnmarshalBlockOrPanic(sn.Data)
			c.lastSnapBlockNum = b.Header.Number
			c.confState = sn.Metadata.ConfState
			c.appliedIndex = sn.Metadata.Index

			if err := c.catchUp(sn); err != nil {
				c.logger.Errorf("Failed to recover from snapshot taken at Term %d and Index %d: %s",
					sn.Metadata.Term, sn.Metadata.Index, err)
			}

		case <-c.doneC:
			select {
			case <-c.errorC: // avoid closing closed channel
			default:
				close(c.errorC)
			}

			c.logger.Infof("Stop serving requests")
			return
		}
	}
}

func (c *Chain) writeBlock(block *common.Block, index uint64) {
	if c.blockInflight > 0 {
		c.blockInflight-- // only reduce on leader
	}

	c.logger.Debugf("Writing block %d to ledger, there are %d blocks in flight", block.Header.Number, c.blockInflight)

	if utils.IsConfigBlock(block) {
		c.writeConfigBlock(block, index)
		return
	}

	c.raftMetadataLock.Lock()
	c.opts.RaftMetadata.RaftIndex = index
	m := utils.MarshalOrPanic(c.opts.RaftMetadata)
	c.raftMetadataLock.Unlock()

	c.support.WriteBlock(block, m)
}

// Orders the envelope in the `msg` content. SubmitRequest.
// Returns
//   -- batches [][]*common.Envelope; the batches cut,
//   -- pending bool; if there are envelopes pending to be ordered,
//   -- err error; the error encountered, if any.
// It takes care of config messages as well as the revalidation of messages if the config sequence has advanced.
func (c *Chain) ordered(msg *orderer.SubmitRequest) (batches [][]*common.Envelope, pending bool, err error) {
	seq := c.support.Sequence()

	if c.isConfig(msg.Payload) {
		// ConfigMsg
		if msg.LastValidationSeq < seq {
			msg.Payload, _, err = c.support.ProcessConfigMsg(msg.Payload)
			if err != nil {
				return nil, true, errors.Errorf("bad config message: %s", err)
			}
		}
		batch := c.support.BlockCutter().Cut()
		batches = [][]*common.Envelope{}
		if len(batch) != 0 {
			batches = append(batches, batch)
		}
		batches = append(batches, []*common.Envelope{msg.Payload})
		return batches, false, nil
	}
	// it is a normal message
	if msg.LastValidationSeq < seq {
		if _, err := c.support.ProcessNormalMsg(msg.Payload); err != nil {
			return nil, true, errors.Errorf("bad normal message: %s", err)
		}
	}
	batches, pending = c.support.BlockCutter().Ordered(msg.Payload)
	return batches, pending, nil

}

func (c *Chain) propose(bc *blockCreator, batches ...[]*common.Envelope) {
	for _, batch := range batches {
		b := bc.createNextBlock(batch)
		data := utils.MarshalOrPanic(b)
		if err := c.node.Propose(context.TODO(), data); err != nil {
			c.logger.Errorf("Failed to propose block to raft: %s", err)
			return // don't bother continue proposing next batch
		}

		// if it is config block, then we should wait for the commit of the block
		if utils.IsConfigBlock(b) {
			c.configInflight = true
		}

		c.blockInflight++
		c.logger.Debugf("Proposed block %d to raft consensus, there are %d blocks in flight", b.Header.Number, c.blockInflight)
	}

	return
}

func (c *Chain) catchUp(snap *raftpb.Snapshot) error {
	b, err := utils.UnmarshalBlock(snap.Data)
	if err != nil {
		return errors.Errorf("failed to unmarshal snapshot data to block: %s", err)
	}

	c.logger.Infof("Catching up with snapshot taken at block %d", b.Header.Number)

	next := c.support.Height()
	if next > b.Header.Number {
		c.logger.Warnf("Snapshot is at block %d, local block number is %d, no sync needed", b.Header.Number, next-1)
		return nil
	}

	puller, err := c.createPuller()
	if err != nil {
		return errors.Errorf("failed to create block puller: %s", err)
	}
	defer puller.Close()

	for next <= b.Header.Number {
		block := puller.PullBlock(next)
		if block == nil {
			return errors.Errorf("failed to fetch block %d from cluster", next)
		}
		if utils.IsConfigBlock(block) {
			c.support.WriteConfigBlock(block, nil)
		} else {
			c.support.WriteBlock(block, nil)
		}

		next++
	}

	c.logger.Infof("Finished syncing with cluster up to block %d (incl.)", b.Header.Number)
	return nil
}

func (c *Chain) apply(ents []raftpb.Entry) {
	if len(ents) == 0 {
		return
	}

	if ents[0].Index > c.appliedIndex+1 {
		c.logger.Panicf("first index of committed entry[%d] should <= appliedIndex[%d]+1", ents[0].Index, c.appliedIndex)
	}

	var appliedb uint64
	var position int
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				break
			}

			// We need to strictly avoid re-applying normal entries,
			// otherwise we are writing the same block twice.
			if ents[i].Index <= c.appliedIndex {
				c.logger.Debugf("Received block with raft index (%d) <= applied index (%d), skip", ents[i].Index, c.appliedIndex)
				break
			}

			block := utils.UnmarshalBlockOrPanic(ents[i].Data)
			c.writeBlock(block, ents[i].Index)

			appliedb = block.Header.Number
			position = i
			c.accDataSize += uint32(len(ents[i].Data))

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(ents[i].Data); err != nil {
				c.logger.Warnf("Failed to unmarshal ConfChange data: %s", err)
				continue
			}

			c.confState = *c.node.ApplyConfChange(cc)

			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				c.logger.Infof("Applied config change to add node %d, current nodes in channel: %+v", cc.NodeID, c.confState.Nodes)
			case raftpb.ConfChangeRemoveNode:
				c.logger.Infof("Applied config change to remove node %d, current nodes in channel: %+v", cc.NodeID, c.confState.Nodes)
			default:
				c.logger.Panic("Programming error, encountered unsupported raft config change")
			}

			// This ConfChange was introduced by a previously committed config block,
			// we can now unblock submitC to accept envelopes.
			if c.confChangeInProgress != nil &&
				c.confChangeInProgress.NodeID == cc.NodeID &&
				c.confChangeInProgress.Type == cc.Type {

				if err := c.configureComm(); err != nil {
					c.logger.Panicf("Failed to configure communication: %s", err)
				}

				c.confChangeInProgress = nil
				c.configInflight = false
			}

			if cc.Type == raftpb.ConfChangeRemoveNode && cc.NodeID == c.raftID {
				c.logger.Infof("Current node removed from replica set for channel %s", c.channelID)
				// calling goroutine, since otherwise it will be blocked
				// trying to write into haltC
				go c.Halt()
			}
		}

		if ents[i].Index > c.appliedIndex {
			c.appliedIndex = ents[i].Index
		}
	}

	if appliedb == 0 {
		// no block has been written (appliedb == 0) in this round
		return
	}

	if c.accDataSize >= c.sizeLimit {
		select {
		case c.gcC <- &gc{index: c.appliedIndex, state: c.confState, data: ents[position].Data}:
			c.logger.Infof("Accumulated %d bytes since last snapshot, exceeding size limit (%d bytes), "+
				"taking snapshot at block %d, last snapshotted block number is %d",
				c.accDataSize, c.sizeLimit, appliedb, c.lastSnapBlockNum)
			c.accDataSize = 0
			c.lastSnapBlockNum = appliedb
		default:
			c.logger.Warnf("Snapshotting is in progress, it is very likely that SnapshotInterval is too small")
		}
	}

	return
}

func (c *Chain) gc() {
	for {
		select {
		case g := <-c.gcC:
			c.node.takeSnapshot(g.index, g.state, g.data)
		case <-c.doneC:
			c.logger.Infof("Stop garbage collecting")
			return
		}
	}
}

func (c *Chain) isConfig(env *common.Envelope) bool {
	h, err := utils.ChannelHeader(env)
	if err != nil {
		c.logger.Panicf("failed to extract channel header from envelope")
	}

	return h.Type == int32(common.HeaderType_CONFIG) || h.Type == int32(common.HeaderType_ORDERER_TRANSACTION)
}

func (c *Chain) configureComm() error {
	// Reset unreachable map when communication is reconfigured
	c.node.unreachableLock.Lock()
	c.node.unreachable = make(map[uint64]struct{})
	c.node.unreachableLock.Unlock()

	nodes, err := c.remotePeers()
	if err != nil {
		return err
	}

	c.configurator.Configure(c.channelID, nodes)
	return nil
}

func (c *Chain) remotePeers() ([]cluster.RemoteNode, error) {
	var nodes []cluster.RemoteNode
	for raftID, consenter := range c.opts.RaftMetadata.Consenters {
		// No need to know yourself
		if raftID == c.raftID {
			continue
		}
		serverCertAsDER, err := c.pemToDER(consenter.ServerTlsCert, raftID, "server")
		if err != nil {
			return nil, errors.WithStack(err)
		}
		clientCertAsDER, err := c.pemToDER(consenter.ClientTlsCert, raftID, "client")
		if err != nil {
			return nil, errors.WithStack(err)
		}
		nodes = append(nodes, cluster.RemoteNode{
			ID:            raftID,
			Endpoint:      fmt.Sprintf("%s:%d", consenter.Host, consenter.Port),
			ServerTLSCert: serverCertAsDER,
			ClientTLSCert: clientCertAsDER,
		})
	}
	return nodes, nil
}

func (c *Chain) pemToDER(pemBytes []byte, id uint64, certType string) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		c.logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

// checkConsentersSet validates correctness of the consenters set provided within configuration value
func (c *Chain) checkConsentersSet(configValue *common.ConfigValue) error {
	// read metadata update from configuration
	updatedMetadata, err := MetadataFromConfigValue(configValue)
	if err != nil {
		return err
	}

	c.raftMetadataLock.RLock()
	changes := ComputeMembershipChanges(c.opts.RaftMetadata.Consenters, updatedMetadata.Consenters)
	c.raftMetadataLock.RUnlock()

	if changes.TotalChanges > 1 {
		return errors.New("update of more than one consenters at a time is not supported")
	}

	return nil
}

// writeConfigBlock writes configuration blocks into the ledger in
// addition extracts updates about raft replica set and if there
// are changes updates cluster membership as well
func (c *Chain) writeConfigBlock(block *common.Block, index uint64) {
	metadata, raftMetadata := c.newRaftMetadata(block)

	var changes *MembershipChanges
	if metadata != nil {
		changes = ComputeMembershipChanges(raftMetadata.Consenters, metadata.Consenters)
	}

	confChange := changes.UpdateRaftMetadataAndConfChange(raftMetadata)
	raftMetadata.RaftIndex = index

	raftMetadataBytes := utils.MarshalOrPanic(raftMetadata)
	// write block with metadata
	c.support.WriteConfigBlock(block, raftMetadataBytes)
	c.configInflight = false

	// update membership
	if confChange != nil {
		// ProposeConfChange returns error only if node being stopped.
		// This proposal is dropped by followers because DisableProposalForwarding is enabled.
		if err := c.node.ProposeConfChange(context.TODO(), *confChange); err != nil {
			c.logger.Warnf("Failed to propose configuration update to Raft node: %s", err)
		}

		c.confChangeInProgress = confChange

		c.raftMetadataLock.Lock()
		c.opts.RaftMetadata = raftMetadata
		c.raftMetadataLock.Unlock()

		switch confChange.Type {
		case raftpb.ConfChangeAddNode:
			c.logger.Infof("Config block just committed adds node %d, pause accepting transactions till config change is applied", confChange.NodeID)
		case raftpb.ConfChangeRemoveNode:
			c.logger.Infof("Config block just committed removes node %d, pause accepting transactions till config change is applied", confChange.NodeID)
		default:
			c.logger.Panic("Programming error, encountered unsupported raft config change")
		}

		c.configInflight = true
	}
}

// getInFlightConfChange returns ConfChange in-flight if any.
// It either returns confChangeInProgress if it is not nil, or
// attempts to read ConfChange from last committed block.
func (c *Chain) getInFlightConfChange() *raftpb.ConfChange {
	if c.confChangeInProgress != nil {
		return c.confChangeInProgress
	}

	if c.support.Height() <= 1 {
		return nil // nothing to failover just started the chain
	}
	lastBlock := c.support.Block(c.support.Height() - 1)
	if lastBlock == nil {
		c.logger.Panicf("nil block, failed to read last written block, blockNum = %d, ledger height = %d, raftID = %d", c.support.Height()-1, c.support.Height(), c.raftID)
	}
	if !utils.IsConfigBlock(lastBlock) {
		return nil
	}

	// extract membership mapping from configuration block metadata
	// and compare with Raft configuration
	metadata, err := utils.GetMetadataFromBlock(lastBlock, common.BlockMetadataIndex_ORDERER)
	if err != nil {
		c.logger.Panicf("Error extracting orderer metadata: %+v", err)
	}

	raftMetadata := &etcdraft.RaftMetadata{}
	if err := proto.Unmarshal(metadata.Value, raftMetadata); err != nil {
		c.logger.Panicf("Failed to unmarshal block's metadata: %+v", err)
	}

	// extracting current Raft configuration state
	confState := c.node.ApplyConfChange(raftpb.ConfChange{})

	if len(confState.Nodes) == len(raftMetadata.Consenters) {
		// since configuration change could only add one node or
		// remove one node at a time, if raft nodes state size
		// equal to membership stored in block metadata field,
		// that means everything is in sync and no need to propose
		// update
		return nil
	}

	return ConfChange(raftMetadata, confState)
}

// newRaftMetadata extract raft metadata from the configuration block
func (c *Chain) newRaftMetadata(block *common.Block) (*etcdraft.Metadata, *etcdraft.RaftMetadata) {
	metadata, err := ConsensusMetadataFromConfigBlock(block)
	if err != nil {
		c.logger.Panicf("error reading consensus metadata: %s", err)
	}
	raftMetadata := proto.Clone(c.opts.RaftMetadata).(*etcdraft.RaftMetadata)
	// proto.Clone doesn't copy an empty map, hence need to initialize it after
	// cloning
	if raftMetadata.Consenters == nil {
		raftMetadata.Consenters = map[uint64]*etcdraft.Consenter{}
	}
	return metadata, raftMetadata
}
