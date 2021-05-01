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

package server

import (
	"github.com/dedas111/protocolX/config"
	"github.com/dedas111/protocolX/helpers"
	"github.com/dedas111/protocolX/networker"
	"github.com/dedas111/protocolX/node"
	// "github.com/dedas111/protocolX/sphinx"

	"github.com/golang/protobuf/proto"

	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	
	"sync"
	// "time"
	"math/rand"
)

var (
	assignFlag = []byte{0xa2}
	commFlag   = []byte{0xc6}
	tokenFlag  = []byte{0xa9}
	pullFlag   = []byte{0xff}
	// handledPackets = 0
	relayedPackets = 0
	messageDelivered = 0
)

// type ProviderIt interface {
// 	networker.NetworkServer
// 	networker.NetworkClient
// 	Start() error
// 	GetConfig() config.MixConfig
// }

type ProviderServer struct {
	id   string
	host string
	port string
	*node.Mix
	listener *net.TCPListener

	assignedClients map[string]ClientRecord
	config          config.MixConfig
	
	aPac []node.MixPacket
	mutex sync.Mutex
}

type ClientRecord struct {
	id     string
	host   string
	port   string
	pubKey []byte
	token  []byte
}

// Start function creates the loggers for capturing the info and error logs
// and starts the listening server. Function returns an error
// signaling whether any operation was unsuccessful
func (p *ProviderServer) Start() error {
	p.aPac = make([]node.MixPacket, 0)
	p.run()
	return nil
}

func (p *ProviderServer) GetConfig() config.MixConfig {
	return p.config
}

// Function opens the listener to start listening on provider's host and port
func (p *ProviderServer) run() {

	defer p.listener.Close()
	finish := make(chan bool)

	go func() {
		logLocal.Infof("ProviderServer: Listening on %s", p.host+":"+p.port)
		p.listenForIncomingConnections()
	}()

	go func() {
		logLocal.Infof("ProviderServer: Preparing for relaying")
		p.relayPacket()
	}()
	
	<-finish
}

// Function processes the received sphinx packet, performs the
// unwrapping operation and checks whether the packet should be
// forwarded or stored. If the processing was unsuccessful and error is returned.
func (p *ProviderServer) receivedPacketWithIndex(packet []byte, index int) error {
	newPacket, err := p.ProcessPacketInSameThread(packet)
	if err != nil {
		return err
	}
	p.aPac[index] = *newPacket

	// cPac := make(chan node.MixPacket)
	// errCh := make(chan error)

	// go p.ProcessPacket(packet, cPac, errCh)
	// err := <-errCh
	// if err != nil {
	// 	return err
	// }
	// p.aPac[index] = <-cPac

	return nil
}

// Function processes the received sphinx packet, performs the
// unwrapping operation and checks whether the packet should be
// forwarded or stored. If the processing was unsuccessful and error is returned.
func (p *ProviderServer) receivedPacket(packet []byte) error {
	// logLocal.Info("ProviderServer: Received new sphinx packet")

	// if GetRemainingRoundTime(config.RoundDuration, config.SyncTime) < int64(20 * time.Millisecond) {
	// 	logLocal.Info("Packet dropped, because received at", (time.Now()).String())
	// 	logLocal.Info("Remaining time:", GetRemainingRoundTime(config.RoundDuration, config.SyncTime))
	// 	return nil
	// }

	// newPacket, err := p.ProcessPacketInSameThread(packet)
	// if err != nil {
	// 	return err
	// }

	cPac := make(chan node.MixPacket)
	errCh := make(chan error)

	go p.ProcessPacket(packet, cPac, errCh)
	err := <-errCh
	if err != nil {
		return err
	}

	p.mutex.Lock()
	p.aPac = append(p.aPac, <-cPac)
	p.mutex.Unlock()

	// // p.mutex.Lock()
	// p.aPac = append(p.aPac, <-cPac)
	// // p.mutex.Unlock()
	// // logLocal.Info("ProviderServer: Processed the sphinx packet at round : ", config.GetRound() )
	// // logLocal.Info("ProviderServer: Current clock time : ", (time.Now()).String())
	// // handledPackets = handledPackets +1
	// // logLocal.Info("ProviderServer: Total packets handled = ", handledPackets)
	return nil
}

func (p *ProviderServer) forwardPacket(sphinxPacket []byte, address string) error {
	packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	if err != nil {
		return err
	}

	err = p.send(packetBytes, address)
	if err != nil {
		return err
	}
	// logLocal.Info("ProviderServer: Forwarded sphinx packet")
	return nil
}

// Function opens a connection with selected network address
// and send the passed packet. If connection failed or
// the packet could not be send, an error is returned
func (p *ProviderServer) send(packet []byte, address string) error {

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.Write(packet)
	return nil
}

// Function responsible for running the listening process of the server;
// The servers listener accepts incoming connections and
// passes the incoming packets to the packet handler.
// If the connection could not be accepted an error
// is logged into the log files, but the function is not stopped
func (p *Server) listenForIncomingConnections() {
	var wg sync.WaitGroup
	for i:=0; i<180; i++ {
		wg.Add(1)
		conn, err := p.listener.Accept()

		if err != nil {
			logLocal.WithError(err).Error(err)
			return
		} 
		go func() {
			// logLocal.Infof("Server: Received new connection from %s", conn.RemoteAddr())
			// logLocal.Info("Server: Current round number : ", config.GetRound())
			defer wg.Done()
			// errs := make(chan error, 1)
			err = p.handleConnection(conn)
			if err != nil {
				logLocal.WithError(err).Error(err)
			}
		}()
	}
	wg.Wait()
	logLocal.Info("Server: packets processing done at round : ", config.GetRound() )
	// logLocal.Info("Server: Total packets processed : ", handledPackets )
}

func (p *Server) relayPacket() error {
	for {
		delayBeforeContinute(config.RoundDuration, config.SyncTime)
		packets := make([]node.MixPacket, len(m.aPac))
		m.mutex.Lock()
		copy(packets, m.aPac)
		m.mutex.Unlock()
		// m.mutex.Lock()
		rand.Shuffle(len(packets), func(i, j int) { packets[i], packets[j] = packets[j], packets[i] })
		for _, p := range packets {
			if p.Flag == "\xF1" {
				m.forwardPacket(p.Data, p.Adr.Address)
				packetsRelayed = packetsRelayed +1
				logLocal.Info("MixServer: Total number of packets relayed", packetsRelayed)
			} else {
				logLocal.Info("MixServer: Packet has non-forward flag. Packet dropped")
			}
		}
		m.mutex.Lock()
		m.aPac = m.aPac[0:0]
		m.mutex.Unlock()
	}
}

// HandleConnection handles the received packets; it checks the flag of the
// packet and schedules a corresponding process function and returns an error.
func (p *Server) handleConnection(conn net.Conn) error {

	buff := make([]byte, 1024)
	reqLen, err := conn.Read(buff)
	defer conn.Close()

	if err != nil {
		logLocal.WithError(err).Error(err)
		return err
	}

	var packet config.GeneralPacket
	err = proto.Unmarshal(buff[:reqLen], &packet)
	if err != nil {
		logLocal.WithError(err).Error(err)
		return err
	}

	switch string(packet.Flag) {
	case string(assignFlag):
		err = p.handleAssignRequest(packet.Data)
		if err != nil {
			logLocal.WithError(err).Error(err)
			return err
		}
	case string(commFlag):
		err = p.receivedPacket(packet.Data)
		if err != nil {
			logLocal.WithError(err).Error(err)
			return err
		}
	case string(pullFlag):
		err = p.handlePullRequest(packet.Data)
		if err != nil {
			logLocal.WithError(err).Error(err)
			return err
		}
	default:
		logLocal.Info(packet.Flag)
		logLocal.Info("Server: Packet flag not recognised. Packet dropped")
		return nil
	}
	return nil
}

// RegisterNewClient generates a fresh authentication token and saves it together with client's public configuration data
// in the list of all registered clients. After the client is registered the function creates an inbox directory
// for the client's inbox, in which clients messages will be stored.
func (p *Server) registerNewClient(clientBytes []byte) ([]byte, string, error) {
	var clientConf config.ClientConfig
	err := proto.Unmarshal(clientBytes, &clientConf)
	if err != nil {
		return nil, "", err
	}

	token := helpers.SHA256([]byte("TMP_Token" + clientConf.Id))
	record := ClientRecord{id: clientConf.Id, host: clientConf.Host, port: clientConf.Port, pubKey: clientConf.PubKey, token: token}
	p.assignedClients[clientConf.Id] = record
	address := clientConf.Host + ":" + clientConf.Port

	path := fmt.Sprintf("./inboxes/%s", clientConf.Id)
	exists, err := helpers.DirExists(path)
	if err != nil {
		return nil, "", err
	}
	if exists == false {
		if err := os.MkdirAll(path, 0775); err != nil {
			return nil, "", err
		}
	}

	return token, address, nil
}

// Function is responsible for handling the registration request from the client.
// it registers the client in the list of all registered clients and send
// an authentication token back to the client.
func (p *Server) handleAssignRequest(packet []byte) error {
	logLocal.Info("Server: Received assign request from the client")

	token, adr, err := p.registerNewClient(packet)
	if err != nil {
		return err
	}

	tokenBytes, err := config.WrapWithFlag(tokenFlag, token)
	if err != nil {
		return err
	}
	err = p.send(tokenBytes, adr)
	if err != nil {
		return err
	}
	return nil
}


// AuthenticateUser compares the authentication token received from the client with
// the one stored by the provider. If tokens are the same, it returns true
// and false otherwise.
func (p *Server) authenticateUser(clientId string, clientToken []byte) bool {

	if bytes.Compare(p.assignedClients[clientId].token, clientToken) == 0 {
		return true
	}
	logLocal.Warningf("ProviderServer: Non matching token: %s, %s", p.assignedClients[clientId].token, clientToken)
	return false
}

// NewServer constructs a new server object.
// NewServer returns a new server object and an error.
func NewServer(id string, host string, port string, pubKey []byte, prvKey []byte, pkiPath string) (*Server, error) {
	node := node.NewMix(pubKey, prvKey)
	server := Server{id: id, host: host, port: port, Mix: node, listener: nil}
	server.config = config.MixConfig{Id: server.id, Host: server.host, Port: server.port, PubKey: server.GetPublicKey()}
	server.assignedClients = make(map[string]ClientRecord)

	configBytes, err := proto.Marshal(&server.config)
	if err != nil {
		return nil, err
	}
	err = helpers.AddToDatabase(pkiPath, "Pki", server.id, "Provider", configBytes)
	if err != nil {
		return nil, err
	}

	addr, err := helpers.ResolveTCPAddress(server.host, server.port)
	if err != nil {
		return nil, err
	}
	server.listener, err = net.ListenTCP("tcp", addr)

	if err != nil {
		return nil, err
	}

	return &server, nil
}
