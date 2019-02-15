/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package multichannel tracks the channel resources for the orderer.  It initially
// loads the set of existing channels, and provides an interface for users of these
// channels to retrieve them, or create new ones.
package multichannel

import (
	"fmt"

	channelconfig "github.com/hyperledger/fabric/common/config/channel"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/orderer/common/ledger"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/common/crypto"
)

var logger = logging.MustGetLogger("orderer/multichannel")

const (
	msgVersion = int32(0)
	epoch      = 0
)

type configResources struct {
	channelconfig.Resources
}

func (cr *configResources) SharedConfig() channelconfig.Orderer {
	oc, ok := cr.OrdererConfig()
	if !ok {
		logger.Panicf("[channel %s] has no orderer configuration", cr.ConfigtxManager().ChainID())
	}
	return oc
}

type ledgerResources struct {
	*configResources
	ledger.ReadWriter
}

// Registrar serves as a point of access and control for the individual channel resources.
type Registrar struct {
	chains          map[string]*ChainSupport
	consenters      map[string]consensus.Consenter
	ledgerFactory   ledger.Factory
	signer          crypto.LocalSigner
	systemChannelID string
	systemChannel   *ChainSupport
	templator       msgprocessor.ChannelConfigTemplator
}

func getConfigTx(reader ledger.Reader) *cb.Envelope {
	lastBlock := ledger.GetBlock(reader, reader.Height()-1)
	index, err := utils.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		logger.Panicf("Chain did not have appropriately encoded last config in its latest block: %s", err)
	}
	configBlock := ledger.GetBlock(reader, index)
	if configBlock == nil {
		logger.Panicf("Config block does not exist")
	}

	return utils.ExtractEnvelopeOrPanic(configBlock, 0)
}

// NewRegistrar produces an instance of a *Registrar.
func NewRegistrar(ledgerFactory ledger.Factory, consenters map[string]consensus.Consenter, signer crypto.LocalSigner) *Registrar {
	r := &Registrar{
		chains:        make(map[string]*ChainSupport),
		ledgerFactory: ledgerFactory,
		consenters:    consenters,
		signer:        signer,
	}

	existingChains := ledgerFactory.ChainIDs()
	for _, chainID := range existingChains {
		rl, err := ledgerFactory.GetOrCreate(chainID)
		if err != nil {
			logger.Panicf("Ledger factory reported chainID %s but could not retrieve it: %s", chainID, err)
		}
		configTx := getConfigTx(rl)
		if configTx == nil {
			logger.Panic("Programming error, configTx should never be nil here")
		}
		ledgerResources := r.newLedgerResources(configTx)
		chainID := ledgerResources.ConfigtxManager().ChainID()

		if _, ok := ledgerResources.ConsortiumsConfig(); ok {
			if r.systemChannelID != "" {
				logger.Panicf("There appear to be two system chains %s and %s", r.systemChannelID, chainID)
			}
			chain := newChainSupport(
				r,
				ledgerResources,
				consenters,
				signer)
			r.templator = msgprocessor.NewDefaultTemplator(chain)
			chain.Processor = msgprocessor.NewSystemChannel(chain, r.templator, msgprocessor.CreateSystemChannelFilters(r, chain))

			// Retrieve genesis block to log its hash. See FAB-5450 for the purpose
			iter, pos := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}})
			defer iter.Close()
			if pos != uint64(0) {
				logger.Panicf("Error iterating over system channel: '%s', expected position 0, got %d", chainID, pos)
			}
			genesisBlock, status := iter.Next()
			if status != cb.Status_SUCCESS {
				logger.Panicf("Error reading genesis block of system channel '%s'", chainID)
			}
			logger.Infof("Starting system channel '%s' with genesis block hash %x and orderer type %s", chainID, genesisBlock.Header.Hash(), chain.SharedConfig().ConsensusType())

			r.chains[chainID] = chain
			r.systemChannelID = chainID
			r.systemChannel = chain
			// We delay starting this chain, as it might try to copy and replace the chains map via newChain before the map is fully built
			defer chain.start()
		} else {
			logger.Debugf("Starting chain: %s", chainID)
			chain := newChainSupport(
				r,
				ledgerResources,
				consenters,
				signer)
			r.chains[chainID] = chain
			chain.start()
		}

	}

	if r.systemChannelID == "" {
		logger.Panicf("No system chain found.  If bootstrapping, does your system channel contain a consortiums group definition?")
	}

	return r
}

// SystemChannelID returns the ChannelID for the system channel.
func (r *Registrar) SystemChannelID() string {
	return r.systemChannelID
}

// BroadcastChannelSupport returns the message channel header, whether the message is a config update
// and the channel resources for a message or an error if the message is not a message which can
// be processed directly (like CONFIG and ORDERER_TRANSACTION messages)
func (r *Registrar) BroadcastChannelSupport(msg *cb.Envelope) (*cb.ChannelHeader, bool, *ChainSupport, error) {
	chdr, err := utils.ChannelHeader(msg)
	if err != nil {
		return nil, false, nil, fmt.Errorf("could not determine channel ID: %s", err)
	}

	cs, ok := r.chains[chdr.ChannelId]
	if !ok {
		cs = r.systemChannel
	}

	class, err := cs.ClassifyMsg(chdr)
	if err != nil {
		return nil, false, nil, fmt.Errorf("could not classify message: %s", err)
	}

	isConfig := false
	switch class {
	case msgprocessor.ConfigUpdateMsg:
		isConfig = true
	default:
	}

	return chdr, isConfig, cs, nil
}

// GetChain retrieves the chain support for a chain (and whether it exists)
func (r *Registrar) GetChain(chainID string) (*ChainSupport, bool) {
	cs, ok := r.chains[chainID]
	return cs, ok
}

func (r *Registrar) newLedgerResources(configTx *cb.Envelope) *ledgerResources {
	configManager, err := channelconfig.New(configTx, nil)
	if err != nil {
		logger.Panicf("Error creating configtx manager and handlers: %s", err)
	}

	chainID := configManager.ChainID()

	ledger, err := r.ledgerFactory.GetOrCreate(chainID)
	if err != nil {
		logger.Panicf("Error getting ledger for %s", chainID)
	}

	return &ledgerResources{
		configResources: &configResources{Resources: configManager},
		ReadWriter:      ledger,
	}
}

func (r *Registrar) newChain(configtx *cb.Envelope) {
	ledgerResources := r.newLedgerResources(configtx)
	ledgerResources.Append(ledger.CreateNextBlock(ledgerResources, []*cb.Envelope{configtx}))

	// Copy the map to allow concurrent reads from broadcast/deliver while the new chainSupport is
	newChains := make(map[string]*ChainSupport)
	for key, value := range r.chains {
		newChains[key] = value
	}

	cs := newChainSupport(r, ledgerResources, r.consenters, r.signer)
	chainID := ledgerResources.ConfigtxManager().ChainID()

	logger.Infof("Created and starting new chain %s", chainID)

	newChains[string(chainID)] = cs
	cs.start()

	r.chains = newChains
}

// ChannelsCount returns the count of the current total number of channels.
func (r *Registrar) ChannelsCount() int {
	return len(r.chains)
}

// NewChannelConfig produces a new template channel configuration based on the system channel's current config.
func (r *Registrar) NewChannelConfig(envConfigUpdate *cb.Envelope) (configtxapi.Manager, error) {
	return r.templator.NewChannelConfig(envConfigUpdate)
}
