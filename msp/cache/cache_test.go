/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"testing"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mocks"
	msp2 "github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewCacheMsp(t *testing.T) {
	i, err := New(nil)
	assert.Error(t, err)
	assert.Nil(t, i)
	assert.Contains(t, err.Error(), "Invalid passed MSP. It must be different from nil.")

	i, err = New(&mocks.MockMSP{})
	assert.NoError(t, err)
	assert.NotNil(t, i)
}

func TestSetup(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	mockMSP.On("Setup", (*msp2.MSPConfig)(nil)).Return(nil)
	err = i.Setup(nil)
	assert.NoError(t, err)
	mockMSP.AssertExpectations(t)
	assert.Equal(t, 0, i.(*cachedMSP).deserializeIdentityCache.Len())
	assert.Equal(t, 0, i.(*cachedMSP).satisfiesPrincipalCache.Len())
	assert.Equal(t, 0, i.(*cachedMSP).validateIdentityCache.Len())
}

func TestGetType(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	mockMSP.On("GetType").Return(msp.FABRIC)
	assert.Equal(t, msp.FABRIC, i.GetType())
	mockMSP.AssertExpectations(t)
}

func TestGetIdentifier(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	mockMSP.On("GetIdentifier").Return("MSP", nil)
	id, err := i.GetIdentifier()
	assert.NoError(t, err)
	assert.Equal(t, "MSP", id)
	mockMSP.AssertExpectations(t)
}

func TestGetSigningIdentity(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	mockIdentity := &mocks.MockSigningIdentity{Mock: mock.Mock{}, MockIdentity: &mocks.MockIdentity{ID: "Alice"}}
	identifier := &msp.IdentityIdentifier{Mspid: "MSP", Id: "Alice"}
	mockMSP.On("GetSigningIdentity", identifier).Return(mockIdentity, nil)
	id, err := i.GetSigningIdentity(identifier)
	assert.NoError(t, err)
	assert.Equal(t, mockIdentity, id)
	mockMSP.AssertExpectations(t)
}

func TestGetDefaultSigningIdentity(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	mockIdentity := &mocks.MockSigningIdentity{Mock: mock.Mock{}, MockIdentity: &mocks.MockIdentity{ID: "Alice"}}
	mockMSP.On("GetDefaultSigningIdentity").Return(mockIdentity, nil)
	id, err := i.GetDefaultSigningIdentity()
	assert.NoError(t, err)
	assert.Equal(t, mockIdentity, id)
	mockMSP.AssertExpectations(t)
}

func TestGetTLSRootCerts(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	expected := [][]byte{{1}, {2}}
	mockMSP.On("GetTLSRootCerts").Return(expected)
	certs := i.GetTLSRootCerts()
	assert.Equal(t, expected, certs)
}

func TestGetTLSIntermediateCerts(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	expected := [][]byte{{1}, {2}}
	mockMSP.On("GetTLSIntermediateCerts").Return(expected)
	certs := i.GetTLSIntermediateCerts()
	assert.Equal(t, expected, certs)
}

func TestDeserializeIdentity(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	// Check id is cached
	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	serializedIdentity := []byte{1, 2, 3}
	mockMSP.On("DeserializeIdentity", serializedIdentity).Return(mockIdentity, nil)
	id, err := i.DeserializeIdentity(serializedIdentity)
	assert.NoError(t, err)
	assert.Equal(t, mockIdentity, id)
	mockMSP.AssertExpectations(t)
	// Check the cache
	_, ok := i.(*cachedMSP).deserializeIdentityCache.Get(string(serializedIdentity))
	assert.True(t, ok)

	// Check the same object is returned
	id, err = i.DeserializeIdentity(serializedIdentity)
	assert.NoError(t, err)
	assert.True(t, mockIdentity == id)
	mockMSP.AssertExpectations(t)

	// Check id is not cached
	mockIdentity = &mocks.MockIdentity{ID: "Bob"}
	serializedIdentity = []byte{1, 2, 3, 4}
	mockMSP.On("DeserializeIdentity", serializedIdentity).Return(mockIdentity, errors.New("Invalid identity"))
	id, err = i.DeserializeIdentity(serializedIdentity)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid identity")
	mockMSP.AssertExpectations(t)

	_, ok = i.(*cachedMSP).deserializeIdentityCache.Get(string(serializedIdentity))
	assert.False(t, ok)
}

func TestValidate(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	// Check validation is cached
	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Alice"})
	mockMSP.On("Validate", mockIdentity).Return(nil)
	err = i.Validate(mockIdentity)
	assert.NoError(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier := mockIdentity.GetIdentifier()
	key := string(identifier.Mspid + ":" + identifier.Id)
	v, ok := i.(*cachedMSP).validateIdentityCache.Get(string(key))
	assert.True(t, ok)
	assert.True(t, v.(bool))

	// Recheck
	err = i.Validate(mockIdentity)
	assert.NoError(t, err)

	// Check validation is not cached
	mockIdentity = &mocks.MockIdentity{ID: "Bob"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Bob"})
	mockMSP.On("Validate", mockIdentity).Return(errors.New("Invalid identity"))
	err = i.Validate(mockIdentity)
	assert.Error(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier = mockIdentity.GetIdentifier()
	key = string(identifier.Mspid + ":" + identifier.Id)
	_, ok = i.(*cachedMSP).validateIdentityCache.Get(string(key))
	assert.False(t, ok)
}

func TestSatisfiesPrincipal(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	assert.NoError(t, err)

	// Check validation is cached
	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Alice"})
	mockMSPPrincipal := &msp2.MSPPrincipal{PrincipalClassification: msp2.MSPPrincipal_IDENTITY, Principal: []byte{1, 2, 3}}
	mockMSP.On("SatisfiesPrincipal", mockIdentity, mockMSPPrincipal).Return(nil)
	mockMSP.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	err = i.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	assert.NoError(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier := mockIdentity.GetIdentifier()
	identityKey := string(identifier.Mspid + ":" + identifier.Id)
	principalKey := string(mockMSPPrincipal.PrincipalClassification) + string(mockMSPPrincipal.Principal)
	key := identityKey + principalKey
	v, ok := i.(*cachedMSP).satisfiesPrincipalCache.Get(key)
	assert.True(t, ok)
	assert.Nil(t, v)

	// Recheck
	err = i.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	assert.NoError(t, err)

	// Check validation is not cached
	mockIdentity = &mocks.MockIdentity{ID: "Bob"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Bob"})
	mockMSPPrincipal = &msp2.MSPPrincipal{PrincipalClassification: msp2.MSPPrincipal_IDENTITY, Principal: []byte{1, 2, 3, 4}}
	mockMSP.On("SatisfiesPrincipal", mockIdentity, mockMSPPrincipal).Return(errors.New("Invalid"))
	mockMSP.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	err = i.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	assert.Error(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier = mockIdentity.GetIdentifier()
	identityKey = string(identifier.Mspid + ":" + identifier.Id)
	principalKey = string(mockMSPPrincipal.PrincipalClassification) + string(mockMSPPrincipal.Principal)
	key = identityKey + principalKey
	v, ok = i.(*cachedMSP).satisfiesPrincipalCache.Get(key)
	assert.True(t, ok)
	assert.NotNil(t, v)
	assert.Contains(t, "Invalid", v.(error).Error())
}
