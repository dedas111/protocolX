// Copyright 2018 The Loopix-Messaging Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
	Package node implements the core functions for a mix node, which allow to process the received cryptographic packets.
*/
package node

import (
	"loopix-messaging/sphinx"
)

type Mix struct {
	pubKey []byte
	prvKey []byte
}

type MixPacket struct {
	Data []byte
	Adr sphinx.Hop
	Flag string
}

// ProcessPacket performs the processing operation on the received packet, including cryptographic operations and
// extraction of the meta information.
func (m *Mix) ProcessPacket(packet []byte, c chan<- MixPacket, errCh chan<- error) {
	
	nextHop, commands, newPacket, err := sphinx.ProcessSphinxPacket(packet, m.prvKey)
	if err != nil {
		errCh <- err
		return
	}

	timeoutCh := make(chan MixPacket, 1)

	go func(p MixPacket, delay float64) {
		// time.Sleep(time.Second * time.Duration(delay))
		timeoutCh <- p
	}(MixPacket{newPacket, nextHop, string(commands.Flag)}, commands.Delay)

	errCh <- nil
	c <- <-timeoutCh

}

// GetPublicKey returns the public key of the mixnode.
func (m *Mix) GetPublicKey() []byte {
	return m.pubKey
}

// NewMix creates a new instance of Mix struct with given public and private key
func NewMix(pubKey []byte, prvKey []byte) *Mix {
	return &Mix{pubKey: pubKey, prvKey: prvKey}
}
