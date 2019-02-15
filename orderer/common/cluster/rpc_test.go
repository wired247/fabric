/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

func TestRPCChangeDestination(t *testing.T) {
	t.Parallel()
	// We send a Submit() to 2 different nodes - 1 and 2.
	// The first invocation of Submit() establishes a stream with node 1
	// and the second establishes a stream with node 2.
	// We define a mock behavior for only a single invocation of Send() on each
	// of the streams (to node 1 and to node 2), therefore we test that invocation
	// of rpc.SendSubmit to node 2 doesn't send the message to node 1.
	comm := &mocks.Communicator{}

	client1 := &mocks.ClusterClient{}
	client2 := &mocks.ClusterClient{}

	comm.On("Remote", "mychannel", uint64(1)).Return(&cluster.RemoteContext{
		Logger:    flogging.MustGetLogger("test"),
		Client:    client1,
		ProbeConn: func(_ *grpc.ClientConn) error { return nil },
	}, nil)
	comm.On("Remote", "mychannel", uint64(2)).Return(&cluster.RemoteContext{
		Logger:    flogging.MustGetLogger("test"),
		Client:    client2,
		ProbeConn: func(_ *grpc.ClientConn) error { return nil },
	}, nil)

	streamToNode1 := &mocks.StepClient{}
	streamToNode2 := &mocks.StepClient{}
	streamToNode1.On("Context", mock.Anything).Return(context.Background())
	streamToNode2.On("Context", mock.Anything).Return(context.Background())

	client1.On("Step", mock.Anything).Return(streamToNode1, nil).Once()
	client2.On("Step", mock.Anything).Return(streamToNode2, nil).Once()

	rpc := &cluster.RPC{
		Logger:        flogging.MustGetLogger("test"),
		Timeout:       time.Hour,
		StreamsByType: cluster.NewStreamsByType(),
		Channel:       "mychannel",
		Comm:          comm,
	}

	var sent sync.WaitGroup
	sent.Add(2)

	signalSent := func(_ mock.Arguments) {
		sent.Done()
	}
	streamToNode1.On("Send", mock.Anything).Return(nil).Run(signalSent).Once()
	streamToNode2.On("Send", mock.Anything).Return(nil).Run(signalSent).Once()
	streamToNode1.On("Recv").Return(nil, io.EOF)
	streamToNode2.On("Recv").Return(nil, io.EOF)

	rpc.SendSubmit(1, &orderer.SubmitRequest{Channel: "mychannel"})
	rpc.SendSubmit(2, &orderer.SubmitRequest{Channel: "mychannel"})

	sent.Wait()
	streamToNode1.AssertNumberOfCalls(t, "Send", 1)
	streamToNode2.AssertNumberOfCalls(t, "Send", 1)
}

func TestSend(t *testing.T) {
	t.Parallel()
	submitRequest := &orderer.SubmitRequest{Channel: "mychannel"}
	submitResponse := &orderer.StepResponse{
		Payload: &orderer.StepResponse_SubmitRes{
			SubmitRes: &orderer.SubmitResponse{Status: common.Status_SUCCESS},
		},
	}

	consensusRequest := &orderer.ConsensusRequest{
		Channel: "mychannel",
	}

	submitReq := wrapSubmitReq(submitRequest)

	consensusReq := &orderer.StepRequest{
		Payload: &orderer.StepRequest_ConsensusRequest{
			ConsensusRequest: consensusRequest,
		},
	}

	comm := &mocks.Communicator{}
	stream := &mocks.StepClient{}
	client := &mocks.ClusterClient{}

	resetMocks := func() {
		// When a mock invokes a method from a different goroutine,
		// it records this invocation. However - we overwrite the recording
		// in this function.

		// Call a setter method on the mock, so it will
		// lock itself and thus synchronize the recordings
		// being done by goroutines from previous test cases.

		stream.Mock.On("bla")
		stream.Mock = mock.Mock{}
		client.Mock = mock.Mock{}
		comm.Mock = mock.Mock{}
	}

	submit := func(rpc *cluster.RPC) error {
		err := rpc.SendSubmit(1, submitRequest)
		return err
	}

	step := func(rpc *cluster.RPC) error {
		return rpc.SendConsensus(1, consensusRequest)
	}

	for _, testCase := range []struct {
		name           string
		method         func(rpc *cluster.RPC) error
		sendReturns    interface{}
		sendCalledWith *orderer.StepRequest
		receiveReturns []interface{}
		stepReturns    []interface{}
		remoteError    error
		expectedErr    string
	}{
		{
			name:           "Send and Receive submit succeed",
			method:         submit,
			sendReturns:    nil,
			stepReturns:    []interface{}{stream, nil},
			receiveReturns: []interface{}{submitResponse, nil},
			sendCalledWith: submitReq,
		},
		{
			name:           "Send step succeed",
			method:         step,
			sendReturns:    nil,
			stepReturns:    []interface{}{stream, nil},
			sendCalledWith: consensusReq,
		},
		{
			name:           "Send submit fails",
			method:         submit,
			sendReturns:    errors.New("oops"),
			stepReturns:    []interface{}{stream, nil},
			sendCalledWith: submitReq,
			expectedErr:    "stream is aborted",
		},
		{
			name:           "Send step fails",
			method:         step,
			sendReturns:    errors.New("oops"),
			stepReturns:    []interface{}{stream, nil},
			sendCalledWith: consensusReq,
			expectedErr:    "stream is aborted",
		},
		{
			name:        "Remote() fails",
			method:      submit,
			remoteError: errors.New("timed out"),
			stepReturns: []interface{}{stream, nil},
			expectedErr: "timed out",
		},
		{
			name:        "Submit fails with Send",
			method:      submit,
			stepReturns: []interface{}{nil, errors.New("deadline exceeded")},
			expectedErr: "deadline exceeded",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			isSend := testCase.receiveReturns == nil
			defer resetMocks()
			var sent sync.WaitGroup
			sent.Add(1)

			stream.On("Context", mock.Anything).Return(context.Background())
			stream.On("Send", mock.Anything).Run(func(_ mock.Arguments) {
				sent.Done()
			}).Return(testCase.sendReturns)
			stream.On("Recv").Return(testCase.receiveReturns...)
			client.On("Step", mock.Anything).Return(testCase.stepReturns...)
			rm := &cluster.RemoteContext{
				SendBuffSize: 1,
				Logger:       flogging.MustGetLogger("test"),
				ProbeConn:    func(_ *grpc.ClientConn) error { return nil },
				Client:       client,
			}
			defer rm.Abort()
			comm.On("Remote", "mychannel", uint64(1)).Return(rm, testCase.remoteError)

			rpc := &cluster.RPC{
				Logger:        flogging.MustGetLogger("test"),
				Timeout:       time.Hour,
				StreamsByType: cluster.NewStreamsByType(),
				Channel:       "mychannel",
				Comm:          comm,
			}

			var err error

			err = testCase.method(rpc)
			if testCase.remoteError == nil && testCase.stepReturns[1] == nil {
				sent.Wait()
				sent.Add(1)
			}

			if testCase.stepReturns[1] == nil && testCase.remoteError == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, testCase.expectedErr)
			}

			if testCase.remoteError == nil && testCase.expectedErr == "" && isSend {
				stream.AssertCalled(t, "Send", testCase.sendCalledWith)
				// Ensure that if we succeeded - only 1 stream was created despite 2 calls
				// to Send() were made
				err := testCase.method(rpc)
				if testCase.expectedErr == "" {
					sent.Wait()
				}

				assert.NoError(t, err)
				stream.AssertNumberOfCalls(t, "Send", 2)
				client.AssertNumberOfCalls(t, "Step", 1)
			}
		})
	}
}

func TestRPCGarbageCollection(t *testing.T) {
	// Scenario: Send a message to a remote node, and establish a stream
	// while doing it.
	// Afterwards - make that stream be aborted, and send a message to a different
	// remote node.
	// The first stream should be cleaned from the mapping.

	t.Parallel()

	comm := &mocks.Communicator{}
	client := &mocks.ClusterClient{}
	stream := &mocks.StepClient{}

	remote := &cluster.RemoteContext{
		Logger:    flogging.MustGetLogger("test"),
		Client:    client,
		ProbeConn: func(_ *grpc.ClientConn) error { return nil },
	}

	var sent sync.WaitGroup

	defineMocks := func(destination uint64) {
		sent.Add(1)
		comm.On("Remote", "mychannel", destination).Return(remote, nil)
		stream.On("Context", mock.Anything).Return(context.Background())
		client.On("Step", mock.Anything).Return(stream, nil).Once()
		stream.On("Send", mock.Anything).Return(nil).Once().Run(func(_ mock.Arguments) {
			sent.Done()
		})
		stream.On("Recv").Return(nil, nil)
	}

	mapping := cluster.NewStreamsByType()

	rpc := &cluster.RPC{
		Logger:        flogging.MustGetLogger("test"),
		Timeout:       time.Hour,
		StreamsByType: mapping,
		Channel:       "mychannel",
		Comm:          comm,
	}

	defineMocks(1)

	rpc.SendSubmit(1, &orderer.SubmitRequest{Channel: "mychannel"})
	// Wait for the message to arrive
	sent.Wait()
	// Ensure the stream is initialized in the mapping
	assert.Len(t, mapping[cluster.SubmitOperation], 1)
	assert.Equal(t, uint64(1), mapping[cluster.SubmitOperation][1].ID)
	// And the underlying gRPC stream indeed had Send invoked on it.
	stream.AssertNumberOfCalls(t, "Send", 1)

	// Abort all streams we currently have that are associated to the remote.
	remote.Abort()

	// The stream still exists, as it is not cleaned yet.
	assert.Len(t, mapping[cluster.SubmitOperation], 1)
	assert.Equal(t, uint64(1), mapping[cluster.SubmitOperation][1].ID)

	// Prepare for the next transmission.
	defineMocks(2)

	// Send a message to a different node.
	rpc.SendSubmit(2, &orderer.SubmitRequest{Channel: "mychannel"})
	// The mapping should be now cleaned from the previous stream.
	assert.Len(t, mapping[cluster.SubmitOperation], 1)
	assert.Equal(t, uint64(2), mapping[cluster.SubmitOperation][2].ID)
}
