/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package multichannel

import (
	"fmt"

	"github.com/hyperledger/fabric/common/config/channel"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/tools/configtxgen/provisional"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

type mockConsenter struct {
}

func (mc *mockConsenter) HandleChain(support consensus.ConsenterSupport, metadata *cb.Metadata) (consensus.Chain, error) {
	return &mockChain{
		queue:    make(chan *cb.Envelope),
		cutter:   support.BlockCutter(),
		support:  support,
		metadata: metadata,
		done:     make(chan struct{}),
	}, nil
}

type mockChain struct {
	queue    chan *cb.Envelope
	cutter   blockcutter.Receiver
	support  consensus.ConsenterSupport
	metadata *cb.Metadata
	done     chan struct{}
}

func (mch *mockChain) Errored() <-chan struct{} {
	return nil
}

func (mch *mockChain) Order(env *cb.Envelope, configSeq uint64) error {
	mch.queue <- env
	return nil
}

func (mch *mockChain) Configure(configUpdate, config *cb.Envelope, configSeq uint64) error {
	mch.queue <- config
	return nil
}

func (mch *mockChain) Start() {
	go func() {
		defer close(mch.done)
		for {
			msg, ok := <-mch.queue
			if !ok {
				return
			}

			chdr, err := utils.ChannelHeader(msg)
			if err != nil {
				logger.Panicf("If a message has arrived to this point, it should already have had header inspected once: %s", err)
			}

			class, err := mch.support.ClassifyMsg(chdr)
			if err != nil {
				logger.Panicf("If a message has arrived to this point, it should already have been classified once: %s", err)
			}
			switch class {
			case msgprocessor.ConfigUpdateMsg:
				batch := mch.support.BlockCutter().Cut()
				if batch != nil {
					block := mch.support.CreateNextBlock(batch)
					mch.support.WriteBlock(block, nil)
				}

				_, err := mch.support.ProcessNormalMsg(msg)
				if err != nil {
					logger.Warningf("Discarding bad config message: %s", err)
					continue
				}
				block := mch.support.CreateNextBlock([]*cb.Envelope{msg})
				mch.support.WriteConfigBlock(block, nil)
			case msgprocessor.NormalMsg:
				batches, _ := mch.support.BlockCutter().Ordered(msg)
				for _, batch := range batches {
					block := mch.support.CreateNextBlock(batch)
					mch.support.WriteBlock(block, nil)
				}
			default:
				logger.Panicf("Unsupported msg classification: %v", class)
			}
		}
	}()
}

func (mch *mockChain) Halt() {
	close(mch.queue)
}

func makeConfigTx(chainID string, i int) *cb.Envelope {
	group := cb.NewConfigGroup()
	group.Groups[config.OrdererGroupKey] = cb.NewConfigGroup()
	group.Groups[config.OrdererGroupKey].Values[fmt.Sprintf("%d", i)] = &cb.ConfigValue{
		Value: []byte(fmt.Sprintf("%d", i)),
	}
	configTemplate := configtx.NewSimpleTemplate(group)
	configEnv, err := configTemplate.Envelope(chainID)
	if err != nil {
		panic(err)
	}
	return makeConfigTxFromConfigUpdateEnvelope(chainID, configEnv)
}

func wrapConfigTx(env *cb.Envelope) *cb.Envelope {
	result, err := utils.CreateSignedEnvelope(cb.HeaderType_ORDERER_TRANSACTION, provisional.TestChainID, mockCrypto(), env, msgVersion, epoch)
	if err != nil {
		panic(err)
	}
	return result
}

func makeConfigTxFromConfigUpdateEnvelope(chainID string, configUpdateEnv *cb.ConfigUpdateEnvelope) *cb.Envelope {
	configUpdateTx, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, chainID, mockCrypto(), configUpdateEnv, msgVersion, epoch)
	if err != nil {
		panic(err)
	}
	configTx, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, chainID, mockCrypto(), &cb.ConfigEnvelope{
		Config:     &cb.Config{Sequence: 1, ChannelGroup: configtx.UnmarshalConfigUpdateOrPanic(configUpdateEnv.ConfigUpdate).WriteSet},
		LastUpdate: configUpdateTx},
		msgVersion, epoch)
	if err != nil {
		panic(err)
	}
	return configTx
}

func makeNormalTx(chainID string, i int) *cb.Envelope {
	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
				Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
				ChannelId: chainID,
			}),
			SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
		},
		Data: []byte(fmt.Sprintf("%d", i)),
	}
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(payload),
	}
}
