/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	channelconfig "github.com/hyperledger/fabric/common/config/channel"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/crypto"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/tools/configtxgen/provisional"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

type mockSystemChannelSupport struct {
	NewChannelConfigVal *mockconfigtx.Manager
	NewChannelConfigErr error
}

func (mscs *mockSystemChannelSupport) NewChannelConfig(env *cb.Envelope) (configtxapi.Manager, error) {
	return mscs.NewChannelConfigVal, mscs.NewChannelConfigErr
}

func TestProcessSystemChannelNormalMsg(t *testing.T) {
	t.Run("Missing header", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{}
		_, err := NewSystemChannel(ms, mscs, nil).ProcessNormalMsg(&cb.Envelope{})
		assert.NotNil(t, err)
		assert.Regexp(t, "no header was set", err.Error())
	})
	t.Run("Mismatched channel ID", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{}
		_, err := NewSystemChannel(ms, mscs, nil).ProcessNormalMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + ".different",
					}),
				},
			}),
		})
		assert.Equal(t, ErrChannelDoesNotExist, err)
	})
	t.Run("Good", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal: 7,
		}
		cs, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessNormalMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		assert.Nil(t, err)
		assert.Equal(t, ms.SequenceVal, cs)

	})
}

func TestSystemChannelConfigUpdateMsg(t *testing.T) {
	t.Run("Missing header", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{})
		assert.NotNil(t, err)
		assert.Regexp(t, "no header was set", err.Error())
	})
	t.Run("NormalUpdate", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		config, cs, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		assert.NotNil(t, config)
		assert.Equal(t, cs, ms.SequenceVal)
		assert.Nil(t, err)
	})
	t.Run("BadNewChannelConfig", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigErr: fmt.Errorf("An error"),
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, mscs.NewChannelConfigErr, err)
	})
	t.Run("BadProposedUpdate", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Manager{
				ProposeConfigUpdateError: fmt.Errorf("An error"),
			},
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, mscs.NewChannelConfigVal.ProposeConfigUpdateError, err)
	})
	t.Run("BadSignEnvelope", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Manager{},
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Regexp(t, "Marshal called with nil", err)
	})
	t.Run("BadByFilter", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Manager{
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			},
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{RejectRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, RejectRule.Apply(nil), err)
	})
	t.Run("Good", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Manager{
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			},
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		config, cs, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, cs, ms.SequenceVal)
		assert.NotNil(t, config)
		assert.Nil(t, err)
	})
}

type mockDefaultTemplatorSupport struct {
	channelconfig.Resources
}

func (mdts *mockDefaultTemplatorSupport) Signer() crypto.LocalSigner {
	return nil
}

func TestNewChannelConfig(t *testing.T) {
	singleMSPGenesisBlock := provisional.New(genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile)).GenesisBlock()
	ctxm, err := channelconfig.New(utils.ExtractEnvelopeOrPanic(singleMSPGenesisBlock, 0), nil)
	assert.Nil(t, err)

	templator := NewDefaultTemplator(&mockDefaultTemplatorSupport{
		Resources: ctxm,
	})

	t.Run("BadPayload", func(t *testing.T) {
		_, err := templator.NewChannelConfig(&cb.Envelope{Payload: []byte("bad payload")})
		assert.Error(t, err, "Should not be able to create new channel config from bad payload.")
	})

	for _, tc := range []struct {
		name    string
		payload *cb.Payload
		regex   string
	}{
		{
			"BadPayloadData",
			&cb.Payload{
				Data: []byte("bad payload data"),
			},
			"^Failing initial channel config creation because of config update envelope unmarshaling error:",
		},
		{
			"BadConfigUpdate",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: []byte("bad config update envelope data"),
				}),
			},
			"^Failing initial channel config creation because of config update unmarshaling error:",
		},
		{
			"MismatchedChannelID",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							ChannelId: "foo",
						},
					),
				}),
			},
			"mismatched channel IDs",
		},
		{
			"EmptyConfigUpdateWriteSet",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{},
					),
				}),
			},
			"^Config update has an empty writeset$",
		},
		{
			"WriteSetNoGroups",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{},
						},
					),
				}),
			},
			"^Config update has missing application group$",
		},
		{
			"WriteSetNoApplicationGroup",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{},
							},
						},
					),
				}),
			},
			"^Config update has missing application group$",
		},
		{
			"BadWriteSetApplicationGroupVersion",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 100,
									},
								},
							},
						},
					),
				}),
			},
			"^Config update for channel creation does not set application group version to 1,",
		},
		{
			"MissingWriteSetConsortiumValue",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{},
							},
						},
					),
				}),
			},
			"^Consortium config value missing$",
		},
		{
			"BadWriteSetConsortiumValueValue",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: &cb.ConfigValue{
										Value: []byte("bad consortium value"),
									},
								},
							},
						},
					),
				}),
			},
			"^Error reading unmarshaling consortium name:",
		},
		{
			"UnknownConsortiumName",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: &cb.ConfigValue{
										Value: utils.MarshalOrPanic(
											&cb.Consortium{
												Name: "NotTheNameYouAreLookingFor",
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"^Unknown consortium name:",
		},
		{
			"Missing consortium members",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: &cb.ConfigValue{
										Value: utils.MarshalOrPanic(
											&cb.Consortium{
												Name: genesisconfig.SampleConsortiumName,
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"Proposed configuration has no application group members, but consortium contains members",
		},
		{
			"Member not in consortium",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
										Groups: map[string]*cb.ConfigGroup{
											"BadOrgName": &cb.ConfigGroup{},
										},
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: &cb.ConfigValue{
										Value: utils.MarshalOrPanic(
											&cb.Consortium{
												Name: genesisconfig.SampleConsortiumName,
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"Attempted to include a member which is not in the consortium",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := templator.NewChannelConfig(&cb.Envelope{Payload: utils.MarshalOrPanic(tc.payload)})
			if assert.Error(t, err) {
				assert.Regexp(t, tc.regex, err.Error())
			}
		})
	}

	// Successful
	t.Run("Success", func(t *testing.T) {
		createTx, err := channelconfig.MakeChainCreationTransaction("foo", genesisconfig.SampleConsortiumName, nil, genesisconfig.SampleOrgName)
		assert.Nil(t, err)
		_, err = templator.NewChannelConfig(createTx)
		assert.Nil(t, err)
	})
}
