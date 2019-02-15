/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"path"
	"reflect"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/raft"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/inactive"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/pkg/errors"
)

// CreateChainCallback creates a new chain
type CreateChainCallback func()

//go:generate mockery -dir . -name InactiveChainRegistry -case underscore -output mocks

// InactiveChainRegistry registers chains that are inactive
type InactiveChainRegistry interface {
	// TrackChain tracks a chain with the given name, and calls the given callback
	// when this chain should be created.
	TrackChain(chainName string, genesisBlock *common.Block, createChain CreateChainCallback)
}

//go:generate mockery -dir . -name ChainGetter -case underscore -output mocks

// ChainGetter obtains instances of ChainSupport for the given channel
type ChainGetter interface {
	// GetChain obtains the ChainSupport for the given channel.
	// Returns nil, false when the ChainSupport for the given channel
	// isn't found.
	GetChain(chainID string) *multichannel.ChainSupport
}

// Config contains etcdraft configurations
type Config struct {
	WALDir  string // WAL data of <my-channel> is stored in WALDir/<my-channel>
	SnapDir string // Snapshots of <my-channel> are stored in SnapDir/<my-channel>
}

// Consenter implements etddraft consenter
type Consenter struct {
	CreateChain           func(chainName string)
	InactiveChainRegistry InactiveChainRegistry
	Dialer                *cluster.PredicateDialer
	Communication         cluster.Communicator
	*Dispatcher
	Chains         ChainGetter
	Logger         *flogging.FabricLogger
	EtcdRaftConfig Config
	OrdererConfig  localconfig.TopLevel
	Cert           []byte
}

// TargetChannel extracts the channel from the given proto.Message.
// Returns an empty string on failure.
func (c *Consenter) TargetChannel(message proto.Message) string {
	switch req := message.(type) {
	case *orderer.ConsensusRequest:
		return req.Channel
	case *orderer.SubmitRequest:
		return req.Channel
	default:
		return ""
	}
}

// ReceiverByChain returns the MessageReceiver for the given channelID or nil
// if not found.
func (c *Consenter) ReceiverByChain(channelID string) MessageReceiver {
	cs := c.Chains.GetChain(channelID)
	if cs == nil {
		return nil
	}
	if cs.Chain == nil {
		c.Logger.Panicf("Programming error - Chain %s is nil although it exists in the mapping", channelID)
	}
	if etcdRaftChain, isEtcdRaftChain := cs.Chain.(*Chain); isEtcdRaftChain {
		return etcdRaftChain
	}
	c.Logger.Warningf("Chain %s is of type %v and not etcdraft.Chain", channelID, reflect.TypeOf(cs.Chain))
	return nil
}

func (c *Consenter) detectSelfID(consenters map[uint64]*etcdraft.Consenter) (uint64, error) {
	var serverCertificates []string
	for nodeID, cst := range consenters {
		serverCertificates = append(serverCertificates, string(cst.ServerTlsCert))
		if bytes.Equal(c.Cert, cst.ServerTlsCert) {
			return nodeID, nil
		}
	}

	c.Logger.Warning("Could not find", string(c.Cert), "among", serverCertificates)
	return 0, cluster.ErrNotInChannel
}

// HandleChain returns a new Chain instance or an error upon failure
func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *common.Metadata) (consensus.Chain, error) {
	m := &etcdraft.Metadata{}
	if err := proto.Unmarshal(support.SharedConfig().ConsensusMetadata(), m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}

	if m.Options == nil {
		return nil, errors.New("etcdraft options have not been provided")
	}

	// determine raft replica set mapping for each node to its id
	// for newly started chain we need to read and initialize raft
	// metadata by creating mapping between conseter and its id.
	// In case chain has been restarted we restore raft metadata
	// information from the recently committed block meta data
	// field.
	raftMetadata, err := ReadRaftMetadata(metadata, m)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read Raft metadata")
	}

	id, err := c.detectSelfID(raftMetadata.Consenters)
	if err != nil {
		c.InactiveChainRegistry.TrackChain(support.ChainID(), support.Block(0), func() {
			c.CreateChain(support.ChainID())
		})
		return &inactive.Chain{Err: errors.Errorf("channel %s is not serviced by me", support.ChainID())}, nil
	}

	opts := Options{
		RaftID:        id,
		Clock:         clock.NewClock(),
		MemoryStorage: raft.NewMemoryStorage(),
		Logger:        c.Logger,

		TickInterval:    time.Duration(m.Options.TickInterval) * time.Millisecond,
		ElectionTick:    int(m.Options.ElectionTick),
		HeartbeatTick:   int(m.Options.HeartbeatTick),
		MaxInflightMsgs: int(m.Options.MaxInflightMsgs),
		MaxSizePerMsg:   m.Options.MaxSizePerMsg,
		SnapInterval:    m.Options.SnapshotInterval,

		RaftMetadata: raftMetadata,

		WALDir:  path.Join(c.EtcdRaftConfig.WALDir, support.ChainID()),
		SnapDir: path.Join(c.EtcdRaftConfig.SnapDir, support.ChainID()),
	}

	rpc := &cluster.RPC{
		Timeout:       c.OrdererConfig.General.Cluster.RPCTimeout,
		Logger:        c.Logger,
		Channel:       support.ChainID(),
		Comm:          c.Communication,
		StreamsByType: cluster.NewStreamsByType(),
	}
	return NewChain(
		support,
		opts,
		c.Communication,
		rpc,
		func() (BlockPuller, error) { return newBlockPuller(support, c.Dialer, c.OrdererConfig.General.Cluster) },
		nil,
	)
}

// ReadRaftMetadata attempts to read raft metadata from block metadata, if available.
// otherwise, it reads raft metadata from config metadata supplied.
func ReadRaftMetadata(blockMetadata *common.Metadata, configMetadata *etcdraft.Metadata) (*etcdraft.RaftMetadata, error) {
	m := &etcdraft.RaftMetadata{
		Consenters:      map[uint64]*etcdraft.Consenter{},
		NextConsenterId: 1,
	}
	if blockMetadata != nil && len(blockMetadata.Value) != 0 { // we have consenters mapping from block
		if err := proto.Unmarshal(blockMetadata.Value, m); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal block's metadata")
		}
		return m, nil
	}

	// need to read consenters from the configuration
	for _, consenter := range configMetadata.Consenters {
		m.Consenters[m.NextConsenterId] = consenter
		m.NextConsenterId++
	}

	return m, nil
}

// New creates a etcdraft Consenter
func New(
	clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	r *multichannel.Registrar,
	icr InactiveChainRegistry,
) *Consenter {
	logger := flogging.MustGetLogger("orderer.consensus.etcdraft")

	var cfg Config
	if err := viperutil.Decode(conf.Consensus, &cfg); err != nil {
		logger.Panicf("Failed to decode etcdraft configuration: %s", err)
	}

	consenter := &Consenter{
		CreateChain:           r.CreateChain,
		InactiveChainRegistry: icr,
		Cert:                  srvConf.SecOpts.Certificate,
		Logger:                logger,
		Chains:                r,
		EtcdRaftConfig:        cfg,
		OrdererConfig:         *conf,
		Dialer:                clusterDialer,
	}
	consenter.Dispatcher = &Dispatcher{
		Logger:        logger,
		ChainSelector: consenter,
	}

	comm := createComm(clusterDialer, consenter, conf.General.Cluster.SendBufferSize)
	consenter.Communication = comm
	svc := &cluster.Service{
		StepLogger: flogging.MustGetLogger("orderer.common.cluster.step"),
		Logger:     flogging.MustGetLogger("orderer.common.cluster"),
		Dispatcher: comm,
	}
	orderer.RegisterClusterServer(srv.Server(), svc)
	return consenter
}

func createComm(clusterDialer *cluster.PredicateDialer, c *Consenter, sendBuffSize int) *cluster.Comm {
	comm := &cluster.Comm{
		SendBufferSize: sendBuffSize,
		Logger:         flogging.MustGetLogger("orderer.common.cluster"),
		Chan2Members:   make(map[string]cluster.MemberMapping),
		Connections:    cluster.NewConnectionStore(clusterDialer),
		ChanExt:        c,
		H:              c,
	}
	c.Communication = comm
	return comm
}
