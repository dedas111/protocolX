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

	"github.com/golang/protobuf/proto"

	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	
	"sync"
	"time"
	"math/rand"
)

var (
	assignFlag = []byte{0xa2}
	commFlag   = []byte{0xc6}
	tokenFlag  = []byte{0xa9}
	pullFlag   = []byte{0xff}
	handledPackets = 0
	relayedPackets = 0
	messageDelivered = 0
)

type ProviderIt interface {
	networker.NetworkServer
	networker.NetworkClient
	Start() error
	GetConfig() config.MixConfig
}

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
func (p *ProviderServer) receivedPacket(packet []byte) error {
	logLocal.Info("ProviderServer: Received new sphinx packet")

	// if GetRemainingRoundTime(config.RoundDuration, config.SyncTime) < int64(20 * time.Millisecond) {
	// 	logLocal.Info("Packet dropped, because received at", (time.Now()).String())
	// 	logLocal.Info("Remaining time:", GetRemainingRoundTime(config.RoundDuration, config.SyncTime))
	// 	return nil
	// }

	cPac := make(chan node.MixPacket)
	errCh := make(chan error)

	go p.ProcessPacket(packet, cPac, errCh)
	err := <-errCh
	if err != nil {
		return err
	}
	// p.mutex.Lock()
	p.aPac = append(p.aPac, <-cPac)
	// p.mutex.Unlock()
	logLocal.Info("ProviderServer: Processed the sphinx packet at round : ", config.GetRound() )
	logLocal.Info("ProviderServer: Current clock time : ", (time.Now()).String())
	handledPackets = handledPackets +1
	logLocal.Info("ProviderServer: Total packets handled = ", handledPackets)
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
	logLocal.Info("ProviderServer: Forwarded sphinx packet")
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
// The providers listener accepts incoming connections and
// passes the incoming packets to the packet handler.
// If the connection could not be accepted an error
// is logged into the log files, but the function is not stopped
func (p *ProviderServer) listenForIncomingConnections() {
	for {
		conn, err := p.listener.Accept()

		if err != nil {
			logLocal.WithError(err).Error(err)
		} else {
			logLocal.Infof("ProviderServer: Received new connection from %s", conn.RemoteAddr())
			logLocal.Info("ProviderServer: Current round number : ", config.GetRound())
			errs := make(chan error, 1)
			go p.handleConnection(conn, errs)
			err = <-errs
			if err != nil {
				logLocal.WithError(err).Error(err)
			}
		}
	}
}

func (p *ProviderServer) relayPacket() error {
	for {
		delayBeforeContinute(config.RoundDuration, config.SyncTime)
		packets := make([]node.MixPacket, len(p.aPac))
		p.mutex.Lock()
		copy(packets, p.aPac)
		p.mutex.Unlock()
		rand.Shuffle(len(packets), func(i, j int) { packets[i], packets[j] = packets[j], packets[i] })
		for _, pp := range packets {
			var err error
			switch pp.Flag {
			case "\xF1":
				err = p.forwardPacket(pp.Data, pp.Adr.Address)
				if err != nil {
					// p.mutex.Unlock()
					return err
				}
				relayedPackets = relayedPackets +1
				logLocal.Info("ProviderServer: Total number of packets relayed", relayedPackets)
				logLocal.Info("ProviderServer: Relayed the sphinx packet at round : ", config.GetRound() )
			case "\xF0":
				err = p.storeMessage(pp.Data, pp.Adr.Id, "TMP_MESSAGE_ID")
				if err != nil {
					// p.mutex.Unlock()
					return err
				}
			default:
				logLocal.Info("ProviderServer: Sphinx packet flag not recognised")
			}
		}
		p.mutex.Lock()
		p.aPac = p.aPac[0:0]
		p.mutex.Unlock()
	}
}

// HandleConnection handles the received packets; it checks the flag of the
// packet and schedules a corresponding process function and returns an error.
func (p *ProviderServer) handleConnection(conn net.Conn, errs chan<- error) {

	buff := make([]byte, 1024)
	reqLen, err := conn.Read(buff)
	defer conn.Close()

	if err != nil {
		errs <- err
	}

	var packet config.GeneralPacket
	err = proto.Unmarshal(buff[:reqLen], &packet)
	if err != nil {
		errs <- err
	}

	switch string(packet.Flag) {
	case string(assignFlag):
		err = p.handleAssignRequest(packet.Data)
		if err != nil {
			errs <- err
		}
	case string(commFlag):
		err = p.receivedPacket(packet.Data)
		if err != nil {
			errs <- err
		}
	case string(pullFlag):
		err = p.handlePullRequest(packet.Data)
		if err != nil {
			errs <- err
		}
	default:
		logLocal.Info(packet.Flag)
		logLocal.Info("ProviderServer: Packet flag not recognised. Packet dropped")
		errs <- nil
	}
	errs <- nil
}

// RegisterNewClient generates a fresh authentication token and saves it together with client's public configuration data
// in the list of all registered clients. After the client is registered the function creates an inbox directory
// for the client's inbox, in which clients messages will be stored.
func (p *ProviderServer) registerNewClient(clientBytes []byte) ([]byte, string, error) {
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
func (p *ProviderServer) handleAssignRequest(packet []byte) error {
	logLocal.Info("ProviderServer: Received assign request from the client")

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

// Function is responsible for handling the pull request received from the client.
// It first authenticates the client, by checking if the received token is valid.
// If yes, the function triggers the function for checking client's inbox
// and sending buffered messages. Otherwise, an error is returned.
func (p *ProviderServer) handlePullRequest(rqsBytes []byte) error {
	var request config.PullRequest
	err := proto.Unmarshal(rqsBytes, &request)
	if err != nil {
		return err
	}

	logLocal.Infof("ProviderServer: Processing pull request: %s %s", request.ClientId, string(request.Token))

	if p.authenticateUser(request.ClientId, request.Token) == true {
		signal, err := p.fetchMessages(request.ClientId)
		if err != nil {
			return err
		}
		switch signal {
		case "NI":
			logLocal.Info("ProviderServer: Inbox does not exist. Sending signal to client.")
		case "EI":
			logLocal.Info("ProviderServer: Inbox is empty. Sending info to the client.")
		case "SI":
			logLocal.Info("ProviderServer: All messages from the inbox succesfuly sent to the client.")
		}
	} else {
		logLocal.Warning("ProviderServer: Authentication went wrong")
		return errors.New("ProviderServer: authentication went wrong")
	}
	return nil
}

// AuthenticateUser compares the authentication token received from the client with
// the one stored by the provider. If tokens are the same, it returns true
// and false otherwise.
func (p *ProviderServer) authenticateUser(clientId string, clientToken []byte) bool {

	if bytes.Compare(p.assignedClients[clientId].token, clientToken) == 0 {
		return true
	}
	logLocal.Warningf("ProviderServer: Non matching token: %s, %s", p.assignedClients[clientId].token, clientToken)
	return false
}

// FetchMessages fetches messages from the requested inbox.
// FetchMessages checks whether an inbox exists and if it contains
// stored messages. If inbox contains any stored messages, all of them
// are send to the client one by one. FetchMessages returns a code
// signaling whether (NI) inbox does not exist, (EI) inbox is empty,
// (SI) messages were send to the client; and an error.
func (p *ProviderServer) fetchMessages(clientId string) (string, error) {

	path := fmt.Sprintf("./inboxes/%s", clientId)
	exist, err := helpers.DirExists(path)
	if err != nil {
		return "", err
	}
	if exist == false {
		return "NI", nil
	}
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "EI", nil
	}

	for _, f := range files {
		dat, err := ioutil.ReadFile(path + "/" + f.Name())
		if err != nil {
			return "", err
		}

		address := p.assignedClients[clientId].host + ":" + p.assignedClients[clientId].port
		logLocal.Infof("ProviderServer: Found stored message for address %s", address)
		messageDelivered += 1
		logLocal.Info("ProviderServer: Total messages delivered to recipient: ", messageDelivered)
		msgBytes, err := config.WrapWithFlag(commFlag, dat)
		if err != nil {
			return "", err
		}
		err = p.send(msgBytes, address)
		if err != nil {
			return "", err
		}
	}
	return "SI", nil
}

// StoreMessage saves the given message in the inbox defined by the given id.
// If the inbox address does not exist or writing into the inbox was unsuccessful
// the function returns an error
func (p *ProviderServer) storeMessage(message []byte, inboxId string, messageId string) error {
	path := fmt.Sprintf("./inboxes/%s", inboxId)
	fileName := path + "/" + messageId + ".txt"

	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(message)
	if err != nil {
		return err
	}

	logLocal.Infof("ProviderServer: Stored message for %s", inboxId)
	return nil
}

// NewProviderServer constructs a new provider object.
// NewProviderServer returns a new provider object and an error.
func NewProviderServer(id string, host string, port string, pubKey []byte, prvKey []byte, pkiPath string) (*ProviderServer, error) {
	node := node.NewMix(pubKey, prvKey)
	providerServer := ProviderServer{id: id, host: host, port: port, Mix: node, listener: nil}
	providerServer.config = config.MixConfig{Id: providerServer.id, Host: providerServer.host, Port: providerServer.port, PubKey: providerServer.GetPublicKey()}
	providerServer.assignedClients = make(map[string]ClientRecord)

	configBytes, err := proto.Marshal(&providerServer.config)
	if err != nil {
		return nil, err
	}
	err = helpers.AddToDatabase(pkiPath, "Pki", providerServer.id, "Provider", configBytes)
	if err != nil {
		return nil, err
	}

	addr, err := helpers.ResolveTCPAddress(providerServer.host, providerServer.port)
	if err != nil {
		return nil, err
	}
	providerServer.listener, err = net.ListenTCP("tcp", addr)

	if err != nil {
		return nil, err
	}

	return &providerServer, nil
}
