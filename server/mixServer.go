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
	Package server implements the mix server.
*/
package server

import (
	"loopix-messaging/config"
	"loopix-messaging/helpers"
	"loopix-messaging/logging"
	"loopix-messaging/networker"
	"loopix-messaging/node"

	"net"

	"github.com/golang/protobuf/proto"
	
	"sync"
	"time"
	"math/rand"
)

var logLocal = logging.PackageLogger()

type MixServerIt interface {
	networker.NetworkServer
	networker.NetworkClient
	GetConfig() config.MixConfig
	Start() error
}

type MixServer struct {
	id       string
	host     string
	port     string
	listener *net.TCPListener
	*node.Mix

	config config.MixConfig
	
	cPac chan node.MixPacket 
	aPac []node.MixPacket
	mutex sync.Mutex
}

func (m *MixServer) Start() error {
	m.cPac = make(chan node.MixPacket)
	m.aPac = make([]node.MixPacket, 0)
	defer m.run()
	return nil
}

func (m *MixServer) GetConfig() config.MixConfig {
	return m.config
}

func (m *MixServer) receivedPacket(packet []byte) error {
	logLocal.Info("Received new sphinx packet")

	errCh := make(chan error)

	go m.ProcessPacket(packet, m.cPac, errCh)
	m.mutex.Lock()
	m.aPac = append(m.aPac, <-m.cPac)
	err := <-errCh
	if err != nil {
		return err
	}
	m.mutex.Unlock()
	return nil
}

func (m *MixServer) forwardPacket(sphinxPacket []byte, address string) error {
	packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	if err != nil {
		return err
	}
	err = m.send(packetBytes, address)
	if err != nil {
		return err
	}

	return nil
}

func (m *MixServer) send(packet []byte, address string) error {

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(packet)
	if err != nil {
		return err
	}
	return nil
}

func (m *MixServer) run() {

	defer m.listener.Close()
	finish := make(chan bool)

	go func() {
		logLocal.Infof("Listening on %s", m.host+":"+m.port)
		m.listenForIncomingConnections()
	}()

	go func() {
		logLocal.Infof("Preparing for relaying")
		m.relayPacket()
	}()
	
	<-finish
}

func (m *MixServer) listenForIncomingConnections() {
	for {
		conn, err := m.listener.Accept()

		if err != nil {
			logLocal.WithError(err).Error(err)
		} else {
			logLocal.Infof("Received connection from %s", conn.RemoteAddr())
			errs := make(chan error, 1)
			go m.handleConnection(conn, errs)
			err = <-errs
			if err != nil {
				logLocal.WithError(err).Error(err)
			}
		}
	}
}

func (m *MixServer) relayPacket() {
	for {
		delayBeforeContinute(config.RoundDuration, config.SyncTime)
		m.mutex.Lock()
		rand.Shuffle(len(m.aPac), func(i, j int) { m.aPac[i], m.aPac[j] = m.aPac[j], m.aPac[i] })
		for _, p := range m.aPac {
			if p.Flag == "\xF1" {
				m.forwardPacket(p.Data, p.Adr.Address)
			} else {
				logLocal.Info("Packet has non-forward flag. Packet dropped")
			}
		}
		m.aPac = m.aPac[0:0]
		m.mutex.Unlock()
	}
}

func delayBeforeContinute(roundDuration time.Duration, syncTime time.Time) error {
	currentTime := time.Now()
	nextRoundTime := syncTime.Add(currentTime.Sub(syncTime).Truncate(roundDuration)).Add(roundDuration)
	time.Sleep(nextRoundTime.Sub(currentTime))
	return nil
}

func (m *MixServer) handleConnection(conn net.Conn, errs chan<- error) {
	defer conn.Close()

	buff := make([]byte, 1024)
	reqLen, err := conn.Read(buff)
	if err != nil {
		errs <- err
	}

	var packet config.GeneralPacket
	err = proto.Unmarshal(buff[:reqLen], &packet)
	if err != nil {
		errs <- err
	}

	switch string(packet.Flag) {
	case string(commFlag):
		err = m.receivedPacket(packet.Data)
		if err != nil {
			errs <- err
		}
	default:
		logLocal.Infof("Packet flag %s not recognised. Packet dropped", packet.Flag)
		errs <- nil
	}
	errs <- nil
}

func NewMixServer(id, host, port string, pubKey []byte, prvKey []byte, pkiPath string) (*MixServer, error) {
	mix := node.NewMix(pubKey, prvKey)
	mixServer := MixServer{id: id, host: host, port: port, Mix: mix, listener: nil}
	mixServer.config = config.MixConfig{Id: mixServer.id, Host: mixServer.host, Port: mixServer.port, PubKey: mixServer.GetPublicKey()}

	configBytes, err := proto.Marshal(&mixServer.config)
	if err != nil {
		return nil, err
	}
	err = helpers.AddToDatabase(pkiPath, "Pki", mixServer.id, "Mix", configBytes)
	if err != nil {
		return nil, err
	}

	addr, err := helpers.ResolveTCPAddress(mixServer.host, mixServer.port)

	if err != nil {
		return nil, err
	}
	mixServer.listener, err = net.ListenTCP("tcp", addr)

	if err != nil {
		return nil, err
	}

	return &mixServer, nil
}
