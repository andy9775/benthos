// Copyright (c) 2014 Ashley Jeffs
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

package output

import (
	"strings"
	"sync/atomic"
	"time"

	"nanomsg.org/go-mangos"
	"nanomsg.org/go-mangos/protocol/pub"
	"nanomsg.org/go-mangos/protocol/push"
	"nanomsg.org/go-mangos/transport/ipc"
	"nanomsg.org/go-mangos/transport/tcp"

	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/response"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

func init() {
	Constructors["nanomsg"] = TypeSpec{
		constructor: NewNanomsg,
		description: `
The scalability protocols are common communication patterns. This output should
be compatible with any implementation, but specifically targets Nanomsg.

Currently only PUSH and PUB sockets are supported.`,
	}
}

//------------------------------------------------------------------------------

// NanomsgConfig contains configuration fields for the Nanomsg output type.
type NanomsgConfig struct {
	URLs          []string `json:"urls" yaml:"urls"`
	Bind          bool     `json:"bind" yaml:"bind"`
	SocketType    string   `json:"socket_type" yaml:"socket_type"`
	PollTimeoutMS int      `json:"poll_timeout_ms" yaml:"poll_timeout_ms"`
}

// NewNanomsgConfig creates a new NanomsgConfig with default values.
func NewNanomsgConfig() NanomsgConfig {
	return NanomsgConfig{
		URLs:          []string{"tcp://localhost:5556"},
		Bind:          false,
		SocketType:    "PUSH",
		PollTimeoutMS: 5000,
	}
}

//------------------------------------------------------------------------------

// Nanomsg is an output type that serves Nanomsg messages.
type Nanomsg struct {
	running int32

	log   log.Modular
	stats metrics.Type

	urls []string
	conf Config

	socket mangos.Socket

	transactions <-chan types.Transaction

	closedChan chan struct{}
	closeChan  chan struct{}
}

// NewNanomsg creates a new Nanomsg output type.
func NewNanomsg(conf Config, mgr types.Manager, log log.Modular, stats metrics.Type) (Type, error) {
	s := Nanomsg{
		running:    1,
		log:        log.NewModule(".output.nanomsg"),
		stats:      stats,
		conf:       conf,
		closedChan: make(chan struct{}),
		closeChan:  make(chan struct{}),
	}
	for _, u := range conf.Nanomsg.URLs {
		for _, splitU := range strings.Split(u, ",") {
			if len(splitU) > 0 {
				s.urls = append(s.urls, splitU)
			}
		}
	}

	var err error
	s.socket, err = getSocketFromType(conf.Nanomsg.SocketType)
	if nil != err {
		return nil, err
	}

	// Set timeout to prevent endless lock.
	err = s.socket.SetOption(
		mangos.OptionRecvDeadline,
		time.Millisecond*time.Duration(s.conf.Nanomsg.PollTimeoutMS),
	)
	if nil != err {
		return nil, err
	}

	s.socket.AddTransport(ipc.NewTransport())
	s.socket.AddTransport(tcp.NewTransport())

	if s.conf.Nanomsg.Bind {
		for _, addr := range s.urls {
			if err = s.socket.Listen(addr); err != nil {
				break
			}
		}
	} else {
		for _, addr := range s.urls {
			if err = s.socket.Dial(addr); err != nil {
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}

	return &s, nil
}

//------------------------------------------------------------------------------

// getSocketFromType returns a socket based on a socket type string.
func getSocketFromType(t string) (mangos.Socket, error) {
	switch t {
	case "PUSH":
		return push.NewSocket()
	case "PUB":
		return pub.NewSocket()
	}
	return nil, types.ErrInvalidScaleProtoType
}

//------------------------------------------------------------------------------

func (s *Nanomsg) loop() {
	var (
		mRunning  = s.stats.GetCounter("output.nanomsg.running")
		mCount    = s.stats.GetCounter("output.nanomsg.count")
		mSendErr  = s.stats.GetCounter("output.nanomsg.send.error")
		mSendSucc = s.stats.GetCounter("output.nanomsg.send.success")
	)

	defer func() {
		atomic.StoreInt32(&s.running, 0)

		s.socket.Close()
		mRunning.Decr(1)

		close(s.closedChan)
	}()
	mRunning.Incr(1)

	if s.conf.Nanomsg.Bind {
		s.log.Infof(
			"Sending nanomsg messages to bound URLs: %s\n",
			s.urls,
		)
	} else {
		s.log.Infof(
			"Sending nanomsg messages to connected URLs: %s\n",
			s.urls,
		)
	}

	var open bool
	for atomic.LoadInt32(&s.running) == 1 {
		var ts types.Transaction
		select {
		case ts, open = <-s.transactions:
			if !open {
				return
			}
		case <-s.closeChan:
			return
		}
		mCount.Incr(1)
		var err error
		for _, part := range ts.Payload.GetAll() {
			if err = s.socket.Send(part); err != nil {
				break
			}
		}
		if err != nil {
			mSendErr.Incr(1)
		} else {
			mSendSucc.Incr(1)
		}
		select {
		case ts.ResponseChan <- response.NewError(err):
		case <-s.closeChan:
			return
		}
	}
}

// Consume assigns a messages channel for the output to read.
func (s *Nanomsg) Consume(ts <-chan types.Transaction) error {
	if s.transactions != nil {
		return types.ErrAlreadyStarted
	}
	s.transactions = ts
	go s.loop()
	return nil
}

// CloseAsync shuts down the Nanomsg output and stops processing messages.
func (s *Nanomsg) CloseAsync() {
	if atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		close(s.closeChan)
	}
}

// WaitForClose blocks until the Nanomsg output has closed down.
func (s *Nanomsg) WaitForClose(timeout time.Duration) error {
	select {
	case <-s.closedChan:
	case <-time.After(timeout):
		return types.ErrTimeout
	}
	return nil
}

//------------------------------------------------------------------------------
