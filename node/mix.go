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
	"github.com/dedas111/protocolX/sphinx"
	"github.com/dedas111/protocolX/logging"
	// "time"
)

var logLocal = logging.PackageLogger()

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
	
	// logLocal.Info("Mix: Before processing the sphinx packet, time : ", (time.Now()).String())
	nextHop, commands, newPacket, err := sphinx.ProcessSphinxPacket(packet, m.prvKey)
	// logLocal.Info("Mix: After processing the sphinx packet, time : ", (time.Now()).String())
	if err != nil {
		errCh <- err
		return
	}
	
	errCh <- nil
	c <- MixPacket{newPacket, nextHop, string(commands.Flag)}

}

func (m *Mix)ProcessPacketInSameThread(packet []byte) (*MixPacket, error){
	
	// logLocal.Info("Mix: Before processing the sphinx packet, time : ", (time.Now()).String())
	nextHop, commands, newPacket, err := sphinx.ProcessSphinxPacket(packet, m.prvKey)
	// logLocal.Info("Mix: After processing the sphinx packet, time : ", (time.Now()).String())
	if err != nil {
		// errCh <- err
		return nil, err
	}
	
	mixPacket := MixPacket{newPacket, nextHop, string(commands.Flag)}
	return &mixPacket, nil

}

// GetPublicKey returns the public key of the mixnode.
func (m *Mix) GetPublicKey() []byte {
	return m.pubKey
}

// NewMix creates a new instance of Mix struct with given public and private key
func NewMix(pubKey []byte, prvKey []byte) *Mix {
	return &Mix{pubKey: pubKey, prvKey: prvKey}
}
