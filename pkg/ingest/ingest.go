// Package ingest provides functionality for
// updating per chat read models (recent history)
package ingest

import (
	"fmt"
	"io"
	"time"

	"github.com/tonto/gossip/pkg/broker"
)

// New creates new ingest instance
func New(mq MQ, s ChatStore) *Ingest {
	return &Ingest{
		mq:    mq,
		store: s,
	}
}

// Ingest represents chat ingester
type Ingest struct {
	mq    MQ
	store ChatStore
}

// MQ represents ingest message queue interface
type MQ interface {
	SubscribeQueue(string, func(uint64, []byte)) (io.Closer, error)
}

// ChatStore represents chat store interface
type ChatStore interface {
	AppendMessage(string, *broker.Msg) error
}

// Run subscribes to ingest queue group and updates chat read model
func (i *Ingest) Run(id string) (func(), error) {
	closer, err := i.mq.SubscribeQueue(
		"chat."+id,
		func(seq uint64, data []byte) {
			msg, err := broker.DecodeMsg(data)
			if err != nil {
				msg = &broker.Msg{
					From: "ingest",
					Text: "ingest: message unavailable: decoding error",
					Time: time.Now(),
				}
			}

			msg.Seq = seq

			// TODO - If AppendMessage or decode errors out, don't ack
			// Ack only after persisting to store (since you are the only one that got the msg (queue subscription))
			i.store.AppendMessage(id, msg)
		},
	)

	if err != nil {
		return nil, fmt.Errorf("ingest: could not subscribe: %v", err)
	}

	return func() { closer.Close() }, nil
}
