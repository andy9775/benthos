// Copyright (c) 2017 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package input

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/pipeline"
	"github.com/Jeffail/benthos/lib/response"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

type mockInput struct {
	ts chan types.Transaction
}

func (m *mockInput) TransactionChan() <-chan types.Transaction {
	return m.ts
}

func (m *mockInput) CloseAsync() {
	close(m.ts)
}

func (m *mockInput) WaitForClose(time.Duration) error {
	return errors.New("wasnt expecting to ever see this tbh")
}

//------------------------------------------------------------------------------

type mockPipe struct {
	tsIn <-chan types.Transaction
	ts   chan types.Transaction
}

func (m *mockPipe) Consume(ts <-chan types.Transaction) error {
	m.tsIn = ts
	return nil
}

func (m *mockPipe) TransactionChan() <-chan types.Transaction {
	return m.ts
}

func (m *mockPipe) CloseAsync() {
	close(m.ts)
}

func (m *mockPipe) WaitForClose(time.Duration) error {
	return nil
}

//------------------------------------------------------------------------------

func TestBasicWrapPipeline(t *testing.T) {
	mockIn := &mockInput{ts: make(chan types.Transaction)}
	mockPi := &mockPipe{
		ts: make(chan types.Transaction),
	}

	newInput, err := WrapWithPipeline(mockIn, func() (types.Pipeline, error) {
		return nil, errors.New("nope")
	})

	if err == nil {
		t.Error("Expected error from back constructor")
	}

	newInput, err = WrapWithPipeline(mockIn, func() (types.Pipeline, error) {
		return mockPi, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if newInput.TransactionChan() != mockPi.ts {
		t.Error("Wrong transaction chan in new input type")
	}

	if mockIn.ts != mockPi.tsIn {
		t.Error("Wrong transactions chan in mock pipe")
	}

	newInput.CloseAsync()
	if err = newInput.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}

	select {
	case _, open := <-mockIn.ts:
		if open {
			t.Error("mock input is still open after close")
		}
	case _, open := <-mockPi.ts:
		if open {
			t.Error("mock pipe is still open after close")
		}
	default:
		t.Error("neither type was closed")
	}
}

func TestWrapZeroPipelines(t *testing.T) {
	mockIn := &mockInput{ts: make(chan types.Transaction)}
	newInput, err := WrapWithPipelines(mockIn)
	if err != nil {
		t.Error(err)
	}

	if newInput != mockIn {
		t.Errorf("Wrong input obj returned: %v != %v", newInput, mockIn)
	}
}

func TestBasicWrapMultiPipelines(t *testing.T) {
	mockIn := &mockInput{ts: make(chan types.Transaction)}
	mockPi1 := &mockPipe{
		ts: make(chan types.Transaction),
	}
	mockPi2 := &mockPipe{
		ts: make(chan types.Transaction),
	}

	newInput, err := WrapWithPipelines(mockIn, func() (types.Pipeline, error) {
		return nil, errors.New("nope")
	})
	if err == nil {
		t.Error("Expected error from back constructor")
	}

	newInput, err = WrapWithPipelines(mockIn, func() (types.Pipeline, error) {
		return mockPi1, nil
	}, func() (types.Pipeline, error) {
		return mockPi2, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if newInput.TransactionChan() != mockPi2.ts {
		t.Error("Wrong message chan in new input type")
	}
	if mockPi2.tsIn != mockPi1.ts {
		t.Error("Wrong message chan in mock pipe 2")
	}

	if mockIn.ts != mockPi1.tsIn {
		t.Error("Wrong messages chan in mock pipe 1")
	}
	if mockPi1.ts != mockPi2.tsIn {
		t.Error("Wrong messages chan in mock pipe 2")
	}

	newInput.CloseAsync()
	if err = newInput.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}

	select {
	case _, open := <-mockIn.ts:
		if open {
			t.Error("mock input is still open after close")
		}
	case _, open := <-mockPi1.ts:
		if open {
			t.Error("mock pipe is still open after close")
		}
	case _, open := <-mockPi2.ts:
		if open {
			t.Error("mock pipe is still open after close")
		}
	default:
		t.Error("neither type was closed")
	}
}

//------------------------------------------------------------------------------

type mockProc struct {
	value string
}

func (m mockProc) ProcessMessage(msg types.Message) ([]types.Message, types.Response) {
	if string(msg.Get(0)) == m.value {
		return nil, response.NewUnack()
	}
	msgs := [1]types.Message{msg}
	return msgs[:], nil
}

//------------------------------------------------------------------------------

func TestBasicWrapProcessors(t *testing.T) {
	mockIn := &mockInput{ts: make(chan types.Transaction)}

	l := log.New(os.Stdout, log.Config{LogLevel: "NONE"})
	s := metrics.DudType{}

	pipe1 := pipeline.NewProcessor(l, s, mockProc{value: "foo"})
	pipe2 := pipeline.NewProcessor(l, s, mockProc{value: "bar"})

	newInput, err := WrapWithPipelines(mockIn, func() (types.Pipeline, error) {
		return pipe1, nil
	}, func() (types.Pipeline, error) {
		return pipe2, nil
	})
	if err != nil {
		t.Error(err)
	}

	resChan := make(chan types.Response)

	msg := message.New([][]byte{[]byte("foo")})

	select {
	case mockIn.ts <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Message should be discarded
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		if !res.SkipAck() {
			t.Error("expected skip ack")
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	msg = message.New([][]byte{[]byte("bar")})

	select {
	case mockIn.ts <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Message should also be discarded
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		if !res.SkipAck() {
			t.Error("expected skip ack")
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	msg = message.New([][]byte{[]byte("baz")})

	select {
	case mockIn.ts <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Message should not be discarded
	var ts types.Transaction
	var open bool
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		t.Errorf("Unexpected response: %v", res.Error())
	case ts, open = <-newInput.TransactionChan():
		if !open {
			t.Error("channel was closed")
		} else if exp, act := "baz", string(ts.Payload.Get(0)); exp != act {
			t.Errorf("Wrong message received: %v != %v", act, exp)
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Send error
	errFailed := errors.New("derp, failed")
	select {
	case ts.ResponseChan <- response.NewError(errFailed):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Receive again
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		t.Errorf("Unexpected response: %v", res.Error())
	case ts, open = <-newInput.TransactionChan():
		if !open {
			t.Error("channel was closed")
		} else if exp, act := "baz", string(ts.Payload.Get(0)); exp != act {
			t.Errorf("Wrong message received: %v != %v", act, exp)
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Send non-error
	select {
	case ts.ResponseChan <- response.NewAck():
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Receive response
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		if res.Error() != nil {
			t.Error(res.Error())
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	newInput.CloseAsync()
	if err = newInput.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}
}

func TestBasicWrapDoubleProcessors(t *testing.T) {
	mockIn := &mockInput{ts: make(chan types.Transaction)}

	l := log.New(os.Stdout, log.Config{LogLevel: "NONE"})
	s := metrics.DudType{}

	pipe1 := pipeline.NewProcessor(l, s, mockProc{value: "foo"}, mockProc{value: "bar"})

	newInput, err := WrapWithPipelines(mockIn, func() (types.Pipeline, error) {
		return pipe1, nil
	})
	if err != nil {
		t.Error(err)
	}

	resChan := make(chan types.Response)

	msg := message.New([][]byte{[]byte("foo")})

	select {
	case mockIn.ts <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Message should be discarded
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		if !res.SkipAck() {
			t.Error("expected skip ack")
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	msg = message.New([][]byte{[]byte("bar")})

	select {
	case mockIn.ts <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Message should also be discarded
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		if !res.SkipAck() {
			t.Error("expected skip ack")
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	msg = message.New([][]byte{[]byte("baz")})

	select {
	case mockIn.ts <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Message should not be discarded
	var ts types.Transaction
	var open bool
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		t.Errorf("Unexpected response: %v", res.Error())
	case ts, open = <-newInput.TransactionChan():
		if !open {
			t.Error("channel was closed")
		} else if exp, act := "baz", string(ts.Payload.Get(0)); exp != act {
			t.Errorf("Wrong message received: %v != %v", act, exp)
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Send error
	errFailed := errors.New("derp, failed")
	select {
	case ts.ResponseChan <- response.NewError(errFailed):
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Receive again
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		t.Errorf("Unexpected response: %v", res.Error())
	case ts, open = <-newInput.TransactionChan():
		if !open {
			t.Error("channel was closed")
		} else if exp, act := "baz", string(ts.Payload.Get(0)); exp != act {
			t.Errorf("Wrong message received: %v != %v", act, exp)
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Send non-error
	select {
	case ts.ResponseChan <- response.NewAck():
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	// Receive response
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Channel was closed")
		}
		if res.Error() != nil {
			t.Error(res.Error())
		}
	case <-time.After(time.Second):
		t.Error("action timed out")
	}

	newInput.CloseAsync()
	if err = newInput.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}
}

//------------------------------------------------------------------------------
