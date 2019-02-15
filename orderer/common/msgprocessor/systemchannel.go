/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"

	channelconfig "github.com/hyperledger/fabric/common/config/channel"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

// ChannelConfigTemplator can be used to generate config templates.
type ChannelConfigTemplator interface {
	// NewChannelConfig creates a new template configuration manager.
	NewChannelConfig(env *cb.Envelope) (configtxapi.Manager, error)
}

// SystemChannel implements the Processor interface for the system channel.
type SystemChannel struct {
	*StandardChannel
	templator ChannelConfigTemplator
}

// NewSystemChannel creates a new system channel message processor.
func NewSystemChannel(support StandardChannelSupport, templator ChannelConfigTemplator, filters *RuleSet) *SystemChannel {
	logger.Debugf("Creating system channel msg processor for channel %s", support.ChainID())
	return &SystemChannel{
		StandardChannel: NewStandardChannel(support, filters),
		templator:       templator,
	}
}

// CreateSystemChannelFilters creates the set of filters for the ordering system chain.
func CreateSystemChannelFilters(chainCreator ChainCreator, ledgerResources channelconfig.Resources) *RuleSet {
	ordererConfig, ok := ledgerResources.OrdererConfig()
	if !ok {
		logger.Panicf("Cannot create system channel filters without orderer config")
	}
	return NewRuleSet([]Rule{
		EmptyRejectRule,
		NewSizeFilter(ordererConfig),
		NewSigFilter(policies.ChannelWriters, ledgerResources.PolicyManager()),
		NewSystemChannelFilter(ledgerResources, chainCreator),
	})
}

// ProcessNormalMsg handles normal messages, rejecting them if they are not bound for the system channel ID
// with ErrChannelDoesNotExist.
func (s *SystemChannel) ProcessNormalMsg(msg *cb.Envelope) (configSeq uint64, err error) {
	channelID, err := utils.ChannelID(msg)
	if err != nil {
		return 0, err
	}

	// For the StandardChannel message processing, we would not check the channel ID,
	// because the message processor is looked up by channel ID.
	// However, the system channel message processor is the catch all for messages
	// which do not correspond to an extant channel, so we must check it here.
	if channelID != s.support.ChainID() {
		return 0, ErrChannelDoesNotExist
	}

	return s.StandardChannel.ProcessNormalMsg(msg)
}

// ProcessConfigUpdateMsg handles messages of type CONFIG_UPDATE either for the system channel itself
// or, for channel creation.  In the channel creation case, the CONFIG_UPDATE is wrapped into a resulting
// ORDERER_TRANSACTION, and in the standard CONFIG_UPDATE case, a resulting CONFIG message
func (s *SystemChannel) ProcessConfigUpdateMsg(envConfigUpdate *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	channelID, err := utils.ChannelID(envConfigUpdate)
	if err != nil {
		return nil, 0, err
	}

	logger.Debugf("Processing config update tx with system channel message processor for channel ID %s", channelID)

	if channelID == s.support.ChainID() {
		return s.StandardChannel.ProcessConfigUpdateMsg(envConfigUpdate)
	}

	// XXX we should check that the signature on the outer envelope is at least valid for some MSP in the system channel

	logger.Debugf("Processing channel create tx for channel %s on system channel %s", channelID, s.support.ChainID())

	// If the channel ID does not match the system channel, then this must be a channel creation transaction

	ctxm, err := s.templator.NewChannelConfig(envConfigUpdate)
	if err != nil {
		return nil, 0, err
	}

	newChannelConfigEnv, err := ctxm.ProposeConfigUpdate(envConfigUpdate)
	if err != nil {
		return nil, 0, err
	}

	newChannelEnvConfig, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID, s.support.Signer(), newChannelConfigEnv, msgVersion, epoch)
	if err != nil {
		return nil, 0, err
	}

	wrappedOrdererTransaction, err := utils.CreateSignedEnvelope(cb.HeaderType_ORDERER_TRANSACTION, s.support.ChainID(), s.support.Signer(), newChannelEnvConfig, msgVersion, epoch)
	if err != nil {
		return nil, 0, err
	}

	// We re-apply the filters here, especially for the size filter, to ensure that the transaction we
	// just constructed is not too large for our consenter.  It additionally reapplies the signature
	// check, which although not strictly necessary, is a good sanity check, in case the orderer
	// has not been configured with the right cert material.  The additional overhead of the signature
	// check is negligable, as this is the channel creation path and not the normal path.
	err = s.StandardChannel.filters.Apply(wrappedOrdererTransaction)
	if err != nil {
		return nil, 0, err
	}

	return wrappedOrdererTransaction, s.support.Sequence(), nil
}

// DefaultTemplatorSupport is the subset of the channel config required by the DefaultTemplator.
type DefaultTemplatorSupport interface {
	// ConsortiumsConfig returns the ordering system channel's Consortiums config.
	ConsortiumsConfig() (channelconfig.Consortiums, bool)

	// ConfigtxManager returns the configtx manager corresponding to the system channel's current config.
	ConfigtxManager() configtxapi.Manager

	// Signer returns the local signer suitable for signing forwarded messages.
	Signer() crypto.LocalSigner
}

// DefaultTemplator implements the ChannelConfigTemplator interface and is the one used in production deployments.
type DefaultTemplator struct {
	support DefaultTemplatorSupport
}

// NewDefaultTemplator returns an instance of the DefaultTemplator.
func NewDefaultTemplator(support DefaultTemplatorSupport) *DefaultTemplator {
	return &DefaultTemplator{
		support: support,
	}
}

// NewChannelConfig creates a new template channel configuration based on the current config in the ordering system channel.
func (dt *DefaultTemplator) NewChannelConfig(envConfigUpdate *cb.Envelope) (configtxapi.Manager, error) {
	configUpdatePayload, err := utils.UnmarshalPayload(envConfigUpdate.Payload)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of payload unmarshaling error: %s", err)
	}

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(configUpdatePayload.Data)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of config update envelope unmarshaling error: %s", err)
	}

	if configUpdatePayload.Header == nil {
		return nil, fmt.Errorf("Failed initial channel config creation because config update header was missing")
	}

	channelHeader, err := utils.UnmarshalChannelHeader(configUpdatePayload.Header.ChannelHeader)
	if err != nil {
		return nil, fmt.Errorf("Failed initial channel config creation because channel header was malformed: %s", err)
	}

	configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of config update unmarshaling error: %s", err)
	}

	if configUpdate.ChannelId != channelHeader.ChannelId {
		return nil, fmt.Errorf("Failing initial channel config creation: mismatched channel IDs: '%s' != '%s'", configUpdate.ChannelId, channelHeader.ChannelId)
	}

	if configUpdate.WriteSet == nil {
		return nil, fmt.Errorf("Config update has an empty writeset")
	}

	if configUpdate.WriteSet.Groups == nil || configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey] == nil {
		return nil, fmt.Errorf("Config update has missing application group")
	}

	if uv := configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Version; uv != 1 {
		return nil, fmt.Errorf("Config update for channel creation does not set application group version to 1, was %d", uv)
	}

	consortiumConfigValue, ok := configUpdate.WriteSet.Values[channelconfig.ConsortiumKey]
	if !ok {
		return nil, fmt.Errorf("Consortium config value missing")
	}

	consortium := &cb.Consortium{}
	err = proto.Unmarshal(consortiumConfigValue.Value, consortium)
	if err != nil {
		return nil, fmt.Errorf("Error reading unmarshaling consortium name: %s", err)
	}

	applicationGroup := cb.NewConfigGroup()
	consortiumsConfig, ok := dt.support.ConsortiumsConfig()
	if !ok {
		return nil, fmt.Errorf("The ordering system channel does not appear to support creating channels")
	}

	consortiumConf, ok := consortiumsConfig.Consortiums()[consortium.Name]
	if !ok {
		return nil, fmt.Errorf("Unknown consortium name: %s", consortium.Name)
	}

	applicationGroup.Policies[channelconfig.ChannelCreationPolicyKey] = &cb.ConfigPolicy{
		Policy: consortiumConf.ChannelCreationPolicy(),
	}
	applicationGroup.ModPolicy = channelconfig.ChannelCreationPolicyKey

	// Get the current system channel config
	systemChannelGroup := dt.support.ConfigtxManager().ConfigEnvelope().Config.ChannelGroup

	// If the consortium group has no members, allow the source request to have no members.  However,
	// if the consortium group has any members, there must be at least one member in the source request
	if len(systemChannelGroup.Groups[channelconfig.ConsortiumsGroupKey].Groups[consortium.Name].Groups) > 0 &&
		len(configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups) == 0 {
		return nil, fmt.Errorf("Proposed configuration has no application group members, but consortium contains members")
	}

	// If the consortium has no members, allow the source request to contain arbitrary members
	// Otherwise, require that the supplied members are a subset of the consortium members
	if len(systemChannelGroup.Groups[channelconfig.ConsortiumsGroupKey].Groups[consortium.Name].Groups) > 0 {
		for orgName := range configUpdate.WriteSet.Groups[channelconfig.ApplicationGroupKey].Groups {
			consortiumGroup, ok := systemChannelGroup.Groups[channelconfig.ConsortiumsGroupKey].Groups[consortium.Name].Groups[orgName]
			if !ok {
				return nil, fmt.Errorf("Attempted to include a member which is not in the consortium")
			}
			applicationGroup.Groups[orgName] = consortiumGroup
		}
	}

	channelGroup := cb.NewConfigGroup()

	// Copy the system channel Channel level config to the new config
	for key, value := range systemChannelGroup.Values {
		channelGroup.Values[key] = value
		if key == channelconfig.ConsortiumKey {
			// Do not set the consortium name, we do this later
			continue
		}
	}

	for key, policy := range systemChannelGroup.Policies {
		channelGroup.Policies[key] = policy
	}

	// Set the new config orderer group to the system channel orderer group and the application group to the new application group
	channelGroup.Groups[channelconfig.OrdererGroupKey] = systemChannelGroup.Groups[channelconfig.OrdererGroupKey]
	channelGroup.Groups[channelconfig.ApplicationGroupKey] = applicationGroup
	channelGroup.Values[channelconfig.ConsortiumKey] = channelconfig.TemplateConsortium(consortium.Name).Values[channelconfig.ConsortiumKey]

	templateConfig, _ := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, configUpdate.ChannelId, dt.support.Signer(), &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: channelGroup,
		},
	}, msgVersion, epoch)

	return channelconfig.NewWithOneTimeSuppressedPolicyWarnings(templateConfig, nil)
}
