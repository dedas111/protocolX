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
	Package client implements the class of a network client which can interact with a mix network.
*/

package client

import (
	"github.com/dedas111/protocolX/clientCore"
	"github.com/dedas111/protocolX/config"
	"github.com/dedas111/protocolX/helpers"
	"github.com/dedas111/protocolX/logging"
	"github.com/dedas111/protocolX/networker"

	"github.com/golang/protobuf/proto"

	"crypto/elliptic"
	"crypto/rand"
	"math/big"
	"net"
	
	// "sync"
	"time"
)

var (
	logLocal                = logging.PackageLogger()
	loopCoverTrafficEnabled = false
	dropCoverTrafficEnabled = false
	assignFlag              = []byte{0xA2}
	commFlag                = []byte{0xc6}
	tokenFlag               = []byte{0xa9}
	pullFlag                = []byte{0xff}
	packetsSent				= 0
	// isSender 				= true
)

type Client interface {
	networker.NetworkClient
	networker.NetworkServer

	Start() error
	SendMessage(message string, recipient config.ClientConfig) error
	ReadInNetworkFromPKI(pkiName string) error
}

type client struct {
	id   string
	host string
	port string

	listener *net.TCPListener
	pkiDir   string

	config config.ClientConfig
	token  []byte

	outQueue         chan []byte
	registrationDone chan bool

	*clientCore.CryptoClient
}

// Start function creates the loggers for capturing the info and error logs;
// it reads the network and users information from the PKI database
// and starts the listening server. Function returns an error
// signaling whenever any operation was unsuccessful.
func (c *client) Start() error {

	c.resolveAddressAndStartListening()

	c.outQueue = make(chan []byte)
	c.registrationDone = make(chan bool)

	err := c.ReadInNetworkFromPKI(c.pkiDir)
	if err != nil {
		logLocal.WithError(err).Error("Error during reading in network PKI")
		return err
	}

	go func() {
		for {
			select {
			case <-c.registrationDone:
				return
			default:
				err = c.sendRegisterMessageToProvider()
				if err != nil {
					logLocal.WithError(err).Error("Error during registration to provider", err)
				}
				time.Sleep(60 * time.Second)
			}
		}
	}()

	c.startListenerInNewRoutine()
	return nil
}

func (c *client) resolveAddressAndStartListening() error {
	addr, err := helpers.ResolveTCPAddress(c.host, c.port)
	if err != nil {
		return err
	}

	c.listener, err = net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	return nil
}

// SendMessage responsible for sending a real message. Takes as input the message string
// and the public information about the destination.
func (c *client) SendMessage(message string, recipient config.ClientConfig) error {
	packet, err := c.encodeMessage(message, recipient)
	if err != nil {
		logLocal.WithError(err).Error("Error in sending message - encode message returned error")
		return err
	}
	c.outQueue <- packet
	return nil
}

// encodeMessage encapsulates the given message into a sphinx packet destinated for recipient
// and wraps with the flag pointing that it is the communication packet
func (c *client) encodeMessage(message string, recipient config.ClientConfig) ([]byte, error) {
	sphinxPacket, err := c.EncodeMessage(message, recipient)
	if err != nil {
		logLocal.WithError(err).Error("Error in sending message - create sphinx packet returned an error")
		return nil, err
	}

	packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	if err != nil {
		logLocal.WithError(err).Error("Error in sending message - wrap with flag returned an error")
		return nil, err
	}
	return packetBytes, nil
}

// Send opens a connection with selected network address
// and send the passed packet. If connection failed or
// the packet could not be send, an error is returned
func (c *client) send(packet []byte, host string, port string) error {

	conn, err := net.Dial("tcp", host+":"+port)

	if err != nil {
		logLocal.WithError(err).Error("Error in send - dial returned an error")
		return err
	}
	defer conn.Close()

	_, err = conn.Write(packet)
	return err
}

// run opens the listener to start listening on clients host and port
func (c *client) startListenerInNewRoutine() {
	defer c.listener.Close()
	finish := make(chan bool)

	go func() {
		logLocal.Infof("Listening on address %s", c.host+":"+c.port)
		c.listenForIncomingConnections()
	}()

	<-finish
}

// ListenForIncomingConnections responsible for running the listening process of the server;
// The clients listener accepts incoming connections and
// passes the incoming packets to the packet handler.
// If the connection could not be accepted an error
// is logged into the log files, but the function is not stopped
func (c *client) listenForIncomingConnections() {
	for {
		conn, err := c.listener.Accept()

		if err != nil {
			logLocal.WithError(err).Error(err)
		} else {
			go c.handleConnection(conn)
		}
	}
}

// HandleConnection handles the received packets; it checks the flag of the
// packet and schedules a corresponding process function;
// The potential errors are logged into the log files.
func (c *client) handleConnection(conn net.Conn) {

	buff := make([]byte, 1024)
	defer conn.Close()

	reqLen, err := conn.Read(buff)
	if err != nil {
		logLocal.WithError(err).Error("Error while reading incoming connection")
		panic(err)
	}
	var packet config.GeneralPacket
	err = proto.Unmarshal(buff[:reqLen], &packet)
	if err != nil {
		logLocal.WithError(err).Error("Error in unmarshal incoming packet")
	}

	switch string(packet.Flag) {
	case string(tokenFlag):
		c.registerToken(packet.Data)
		if c.id == "0" {
			go func() {
				err := c.controlOutQueue()
				if err != nil {
					logLocal.WithError(err).Panic("Error in the controller of the outgoing packets queue. Possible security threat.")
				}
			}()
		} else {
			go func() {
				c.controlMessagingFetching()
			}()
		}

		// if loopCoverTrafficEnabled {
		// 	c.turnOnLoopCoverTraffic()
		// }

		// if dropCoverTrafficEnabled {
		// 	c.turnOnDropCoverTraffic()
		// }

		// go func() {
		// 	c.controlMessagingFetching()
		// }()

	case string(commFlag):
		_, err := c.processPacket(packet.Data)
		if err != nil {
			logLocal.WithError(err).Error("Error in processing received packet")
		}
		logLocal.Info("Received new message")
	default:
		logLocal.Infof("Packet flag %x not recognised. Packet dropped.", packet.Flag[0])
	}
}

// RegisterToken stores the authentication token received from the provider
func (c *client) registerToken(token []byte) {
	c.token = token
	logLocal.Infof(" Registered token %s", c.token)
	c.registrationDone <- true
}

// ProcessPacket processes the received sphinx packet and returns the
// encapsulated message or error in case the processing
// was unsuccessful.
func (c *client) processPacket(packet []byte) ([]byte, error) {
	logLocal.Info(" Processing packet")
	return packet, nil
}

// SendRegisterMessageToProvider allows the client to register with the selected provider.
// The client sends a special assignment packet, with its public information, to the provider
// or returns an error.
func (c *client) sendRegisterMessageToProvider() error {

	logLocal.Info("Sending request to provider to register")

	confBytes, err := proto.Marshal(&c.config)
	if err != nil {
		logLocal.WithError(err).Error("Error in register provider - marshal of provider config returned an error")
		return err
	}

	pktBytes, err := config.WrapWithFlag(assignFlag, confBytes)
	if err != nil {
		logLocal.WithError(err).Error("Error in register provider - wrap with flag returned an error")
		return err
	}

	err = c.send(pktBytes, c.Provider.Host, c.Provider.Port)
	if err != nil {
		logLocal.WithError(err).Error("Error in register provider - send registration packet returned an error")
		return err
	}
	return nil
}

// GetMessagesFromProvider allows to fetch messages from the inbox stored by the
// provider. The client sends a pull packet to the provider, along with
// the authentication token. An error is returned if occurred.
func (c *client) getMessagesFromProvider() error {
	pullRqs := config.PullRequest{ClientId: c.id, Token: c.token}
	pullRqsBytes, err := proto.Marshal(&pullRqs)
	if err != nil {
		logLocal.WithError(err).Error("Error in register provider - marshal of pull request returned an error")
		return err
	}

	pktBytes, err := config.WrapWithFlag(pullFlag, pullRqsBytes)
	if err != nil {
		logLocal.WithError(err).Error("Error in register provider - marshal of provider config returned an error")
		return err
	}

	err = c.send(pktBytes, c.Provider.Host, c.Provider.Port)
	if err != nil {
		return err
	}

	return nil
}

// controlOutQueue controls the outgoing queue of the client.
// If a message awaits in the queue, it is sent. Otherwise a
// drop cover message is sent instead.
func (c *client) controlOutQueue() error {
	logLocal.Info("Queue controller started")

	// Just to allow the registration thread to continue.
	time.Sleep(1 * time.Millisecond)

	// "I hate this hack." -- Debo.
	dummyQueue := make([][]byte, 200)
	for i:=0; i<200; i++ {
		dummyPacket, err := c.createDropCoverMessage()
		if err != nil {
			logLocal.WithError(err).Error("Error during populating the dummy message queue", err)
		}
		dummyQueue[i] = dummyPacket
		// logLocal.Info("dummyQueue is appended with message number ", i)
	} 

	// dummyPacket, err := c.createDropCoverMessage()
	// if err != nil {
	// 	logLocal.WithError(err).Error("Error during the construction of a dummy packet. ", err)
	// 	return err
	// }

	// var wg sync.WaitGroup

	for j := 0; j < 1; j++{
		err := delayBeforeContinute(config.RoundDuration, config.SyncTime)
		if err != nil {
			return err
		}

		round := config.GetRound()
		logLocal.Info("Client: Running round number ", round)
		logLocal.Info("Client: Current clock time : ", (time.Now()).String())
		
		// "Again, I don't like this hack" -- Debo.
		for i := 0; i < 200; i++ {
			select {
			case realPacket := <-c.outQueue:
				c.send(realPacket, c.Provider.Host, c.Provider.Port)
				logLocal.Info("Real packet was sent at ", (time.Now()).String())
				packetsSent = packetsSent +1
				logLocal.Info("Packet sent = ", packetsSent)
			default:
				// dummyPacket, err := c.createDropCoverMessage()
				// if err != nil {
				// 	return err
				// }
				// wg.Add(1)
				// go func() {
				// 	defer wg.Done()
				// 	c.send(dummyPacket, c.Provider.Host, c.Provider.Port)
				// 	// c.send(dummyQueue[50*j+i], c.Provider.Host, c.Provider.Port)
				// 	logLocal.Info("OutQueue empty. Dummy packet sent at round ", config.GetRound())
				// 	// packetsSent = packetsSent +1
				// 	// logLocal.Info("Packet sent = ", packetsSent)
				// } ()

				dummyPacket := dummyQueue[i]
				err = c.send(dummyPacket, c.Provider.Host, c.Provider.Port)
				// c.send(dummyQueue[50*j+i], c.Provider.Host, c.Provider.Port)
				if err != nil {
					logLocal.Info("OutQueue empty. There was an error sending dummy packet: ", err)
				} else {
					logLocal.Info("OutQueue empty. Dummy packet sent at round ", config.GetRound())
				}
				// logLocal.Info("OutQueue empty. Dummy packet sent at round ", config.GetRound())
			}
			// err := delayBeforeContinute(config.ClientDuration, config.SyncTime)
			// if err != nil {
			// 	return err
			// }
		}

		// wg.Wait()
		roundEnd := config.GetRound()
		logLocal.Info("Client: When done sending messages, the round number is ", roundEnd)
		logLocal.Info("Client: Started at round number ", round)
		logLocal.Info("Client: Current clock time : ", (time.Now()).String())
	}
	return nil
}

// controlMessagingFetching periodically at random sends a query to the provider
// to fetch received messages
func (c *client) controlMessagingFetching() {
	for i := 0; i < 50; i++ {
		c.getMessagesFromProvider()
		logLocal.Info("Sent request to provider to fetch messages")
		err := delayBeforeContinute(config.RoundDuration, config.SyncTime)
		if err != nil {
			logLocal.Error("Error in ControlMessagingFetching - generating random exp. value failed")
		}
	}
}

// createDummyMessage packs a dummy message into a Sphinx packet.
// The dummy message is a noise message.
func (c *client) createDummyMessage() ([]byte, error) {
	dummyLoad := "DummyPayloadMessage"

	sphinxPacket, err := c.EncodeMessage(dummyLoad, c.getRecipient(c.Network.Clients))
	if err != nil {
		return nil, err
	}

	packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	if err != nil {
		return nil, err
	}
	return packetBytes, nil
}

// CreateCoverMessage packs a dummy message into a Sphinx packet.
// The dummy message is a loop message.
func (c *client) createDropCoverMessage() ([]byte, error) {
	dummyLoad := "DummyPayloadMessage"
	randomRecipient, err := c.getRandomRecipient(c.Network.Clients)
	if err != nil {
		return nil, err
	}
	sphinxPacket, err := c.EncodeMessage(dummyLoad, randomRecipient)
	if err != nil {
		return nil, err
	}

	packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	if err != nil {
		return nil, err
	}
	return packetBytes, nil
}

// getRandomRecipient picks a random client from the list of all available clients (stored by the client).
// getRandomRecipient returns the selected client public configuration and an error
func (c *client) getRecipient(slice []config.ClientConfig) (config.ClientConfig) {
	for _, recipient := range slice {
		if recipient.GetId() != c.id {
			return recipient
		}
	}
	return slice[0]
}

// getRandomRecipient picks a random client from the list of all available clients (stored by the client).
// getRandomRecipient returns the selected client public configuration and an error
func (c *client) getRandomRecipient(slice []config.ClientConfig) (config.ClientConfig, error) {
	randIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(slice))))
	if err != nil {
		return config.ClientConfig{}, err
	}
	for slice[randIdx.Int64()].GetId() == c.id {
		randIdx, err = rand.Int(rand.Reader, big.NewInt(int64(len(slice))))
	}
	return slice[randIdx.Int64()], nil
}

// createLoopCoverMessage packs a dummy loop message into
// a sphinx packet. The loop message is destinated back to the sender
// createLoopCoverMessage returns a byte representation of the encapsulated packet and an error
func (c *client) createLoopCoverMessage() ([]byte, error) {
	loopLoad := "LoopCoverMessage"
	sphinxPacket, err := c.EncodeMessage(loopLoad, c.config)
	if err != nil {
		return nil, err
	}
	packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	if err != nil {
		return nil, err
	}
	return packetBytes, nil
}

// runLoopCoverTrafficStream manages the stream of loop cover traffic.
// In each stream iteration it sends a freshly created loop packet and
// waits a random time before scheduling the next loop packet.
func (c *client) runLoopCoverTrafficStream() error {
	logLocal.Info("Stream of loop cover traffic started")
	for {
		loopPacket, err := c.createLoopCoverMessage()
		if err != nil {
			return err
		}
		c.send(loopPacket, c.Provider.Host, c.Provider.Port)
		logLocal.Info("Loop message sent")
		err = delayBeforeContinute(config.RoundDuration, config.SyncTime)
		if err != nil {
			return err
		}

	}
	return nil
}

// runDropCoverTrafficStream manages the stream of drop cover traffic.
// In each stream iteration it creates a fresh drop cover message destinated
// to a randomly selected user in the network. The drop packet is sent
// and the next stream call is scheduled after random time.
func (c *client) runDropCoverTrafficStream() error {
	logLocal.Info("Stream of drop cover traffic started")
	for {
		dropPacket, err := c.createDropCoverMessage()
		if err != nil {
			return err
		}
		c.send(dropPacket, c.Provider.Host, c.Provider.Port)
		logLocal.Info("Drop packet sent")
		err = delayBeforeContinute(config.RoundDuration, config.SyncTime)
		if err != nil {
			return err
		}
	}
	return nil
}

// delayBeforeContinue waits for the next round based on the system clock.
func delayBeforeContinute(roundDuration time.Duration, syncTime time.Time) error {
	currentTime := time.Now()
	nextRoundTime := syncTime.Add(currentTime.Sub(syncTime).Truncate(roundDuration)).Add(roundDuration)
	time.Sleep(nextRoundTime.Sub(currentTime))
	return nil
}

// turnOnLoopCoverTraffic starts the stream of loop cover traffic
func (c *client) turnOnLoopCoverTraffic() {
	go func() {
		err := c.runLoopCoverTrafficStream()
		if err != nil {
			logLocal.WithError(err).Panic("Error in the controller of the loop cover traffic. Possible security threat.")
		}
	}()
}

// turnOnDropCoverTraffic starts the stream of drop cover traffic
func (c *client) turnOnDropCoverTraffic() {
	go func() {
		err := c.runDropCoverTrafficStream()
		if err != nil {
			logLocal.WithError(err).Panic("Error in the controller of the drop cover traffic. Possible security threat.")
		}
	}()
}

// ReadInNetworkFromPKI reads in the public information about active mixes
// from the PKI database and stores them locally. In case
// the connection or fetching data from the PKI went wrong,
// an error is returned.
func (c *client) ReadInNetworkFromPKI(pkiName string) error {
	logLocal.Infof("Reading network information from the PKI: %s", pkiName)

	mixes, err := helpers.GetMixesPKI(pkiName)
	if err != nil {
		logLocal.WithError(err).Error("Error while reading mixes from PKI")
		return err
	}
	c.Network.Mixes = mixes

	clients, err := helpers.GetClientPKI(pkiName)
	if err != nil {
		logLocal.WithError(err).Error("Error while reading clients from PKI")
		return err
	}
	c.Network.Clients = clients

	logLocal.Info("Network information uploaded")
	return nil
}

// The constructor function to create an new client object.
// Function returns a new client object or an error, if occurred.
func NewClient(id, host, port string, pubKey []byte, prvKey []byte, pkiDir string, provider config.MixConfig) (*client, error) {
	core := clientCore.NewCryptoClient(pubKey, prvKey, elliptic.P224(), provider, clientCore.NetworkPKI{})
	c := client{id: id, host: host, port: port, CryptoClient: core, pkiDir: pkiDir}
	c.config = config.ClientConfig{Id: c.id, Host: c.host, Port: c.port, PubKey: c.GetPublicKey(), Provider: &c.Provider}

	configBytes, err := proto.Marshal(&c.config)

	if err != nil {
		return nil, err
	}
	err = helpers.AddToDatabase(pkiDir, "Pki", c.id, "Client", configBytes)
	if err != nil {
		return nil, err
	}

	// if clientType == "recipient" {
	// 	isSender = false
	// 	logLocal.Info("This is a dedicated recipient, no messages will be sent from this client.")
	// } else {
	// 	logLocal.Info("This client is going to send a lot of messages.")
	// }
	// isSender = senderFlag

	return &c, nil
}

// NewTestClient constructs a client object, which can be used for testing. The object contains the crypto core
// and the top-level of client, but does not involve networking and starting a listener.
func NewTestClient(id, host, port string, pubKey []byte, prvKey []byte, pkiDir string, provider config.MixConfig) (*client, error) {
	core := clientCore.NewCryptoClient(pubKey, prvKey, elliptic.P224(), provider, clientCore.NetworkPKI{})
	c := client{id: id, host: host, port: port, CryptoClient: core, pkiDir: pkiDir}
	c.config = config.ClientConfig{Id: c.id, Host: c.host, Port: c.port, PubKey: c.GetPublicKey(), Provider: &c.Provider}

	return &c, nil
}
