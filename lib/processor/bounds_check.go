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

package processor

import (
	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/response"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

func init() {
	Constructors[TypeBoundsCheck] = TypeSpec{
		constructor: NewBoundsCheck,
		description: `
Checks whether each message fits within certain boundaries, and drops messages
that do not. A metric is incremented for each dropped message and debug logs
are also provided if enabled.`,
	}
}

//------------------------------------------------------------------------------

// BoundsCheckConfig contains configuration fields for the BoundsCheck
// processor.
type BoundsCheckConfig struct {
	MaxParts    int `json:"max_parts" yaml:"max_parts"`
	MinParts    int `json:"min_parts" yaml:"min_parts"`
	MaxPartSize int `json:"max_part_size" yaml:"max_part_size"`
	MinPartSize int `json:"min_part_size" yaml:"min_part_size"`
}

// NewBoundsCheckConfig returns a BoundsCheckConfig with default values.
func NewBoundsCheckConfig() BoundsCheckConfig {
	return BoundsCheckConfig{
		MaxParts:    100,
		MinParts:    1,
		MaxPartSize: 1 * 1024 * 1024 * 1024, // 1GB
		MinPartSize: 1,
	}
}

//------------------------------------------------------------------------------

// BoundsCheck is a processor that checks each message against a set of bounds
// and rejects messages if they aren't within them.
type BoundsCheck struct {
	conf  Config
	log   log.Modular
	stats metrics.Type

	mCount           metrics.StatCounter
	mDropped         metrics.StatCounter
	mDroppedEmpty    metrics.StatCounter
	mDroppedNumParts metrics.StatCounter
	mDroppedPartSize metrics.StatCounter
	mSent            metrics.StatCounter
	mSentParts       metrics.StatCounter
}

// NewBoundsCheck returns a BoundsCheck processor.
func NewBoundsCheck(
	conf Config, mgr types.Manager, log log.Modular, stats metrics.Type,
) (Type, error) {
	return &BoundsCheck{
		conf:  conf,
		log:   log.NewModule(".processor.bounds_check"),
		stats: stats,

		mCount:           stats.GetCounter("processor.bounds_check.count"),
		mDropped:         stats.GetCounter("processor.bounds_check.dropped"),
		mDroppedEmpty:    stats.GetCounter("processor.bounds_check.dropped_empty"),
		mDroppedNumParts: stats.GetCounter("processor.bounds_check.dropped_num_parts"),
		mDroppedPartSize: stats.GetCounter("processor.bounds_check.dropped_part_size"),
		mSent:            stats.GetCounter("processor.bounds_check.sent"),
		mSentParts:       stats.GetCounter("processor.bounds_check.parts.sent"),
	}, nil
}

//------------------------------------------------------------------------------

// ProcessMessage applies the processor to a message, either creating >0
// resulting messages or a response to be sent back to the message source.
func (m *BoundsCheck) ProcessMessage(msg types.Message) ([]types.Message, types.Response) {
	m.mCount.Incr(1)

	lParts := msg.Len()
	if lParts < m.conf.BoundsCheck.MinParts {
		m.log.Debugf(
			"Rejecting message due to message parts below minimum (%v): %v\n",
			m.conf.BoundsCheck.MinParts, lParts,
		)
		m.mDropped.Incr(1)
		m.mDroppedEmpty.Incr(1)
		return nil, response.NewAck()
	} else if lParts > m.conf.BoundsCheck.MaxParts {
		m.log.Debugf(
			"Rejecting message due to message parts exceeding limit (%v): %v\n",
			m.conf.BoundsCheck.MaxParts, lParts,
		)
		m.mDropped.Incr(1)
		m.mDroppedNumParts.Incr(1)
		return nil, response.NewAck()
	}

	for _, part := range msg.GetAll() {
		if size := len(part); size > m.conf.BoundsCheck.MaxPartSize ||
			size < m.conf.BoundsCheck.MinPartSize {
			m.log.Debugf(
				"Rejecting message due to message part size (%v -> %v): %v\n",
				m.conf.BoundsCheck.MinPartSize,
				m.conf.BoundsCheck.MaxPartSize,
				size,
			)
			m.mDropped.Incr(1)
			m.mDroppedPartSize.Incr(1)
			return nil, response.NewAck()
		}
	}

	m.mSent.Incr(1)
	m.mSentParts.Incr(int64(msg.Len()))
	msgs := [1]types.Message{msg}
	return msgs[:], nil
}

//------------------------------------------------------------------------------
