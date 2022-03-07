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
	"fmt"
	"github.com/dedas111/protocolX/config"
	"github.com/dedas111/protocolX/helpers"
	"github.com/dedas111/protocolX/networker"
	"github.com/dedas111/protocolX/node"
	"github.com/dedas111/protocolX/pki"
	"runtime"
	"strings"

	// "github.com/dedas111/protocolX/sphinx"
	"github.com/dedas111/protocolX/logging"

	"github.com/golang/protobuf/proto"

	"crypto/rand"
	"crypto/tls"
	"crypto/x509"

	mrand "math/rand"
	// "bytes"
	// "errors"
	"strconv"
	// "fmt"
	// "io"
	// "io/ioutil"
	"net"
	// "os"
	"sync"
	"time"
)

var (
	assignFlag = []byte{0xa2}
	commFlag   = []byte{0xc6}
	tokenFlag  = []byte{0xa9}
	pullFlag   = []byte{0xff}
	// handledPackets = 0
	relayedPackets   = 0
	messageDelivered = 0
	isMapper         = true
	// array = 0
	msgCount         = 500000
	threadsCount     = 1
	staticServerRole = ""
	lbCtr            = 0
	emptyCtr         = 0

	logLocal = logging.PackageLogger()
)

const (
	PKI_DIR = "pki/database.db"
	// TODO: make this dynamic for example with cmdline parameters (only needed for randomness beacon, remove if changed to other randomness beacon)
	globalNodeCount = 3
)

type ProviderIt interface {
	networker.NetworkServer
	networker.NetworkClient
	Start() error
	GetConfig() config.MixConfig
}

type ConcurrentIndex struct {
	mu    sync.Mutex
	array []int
}

type ConcurrentReceivedPackets struct {
	mu    sync.Mutex
	array [][][]byte
}

type Server struct {
	id   string
	host string
	port string
	*node.Mix
	// listener *net.TCPListener
	listener net.Listener

	assignedClients map[string]ClientRecord
	config          config.MixConfig

	aPac                []node.MixPacket
	mutex               sync.Mutex
	runningIndex        ConcurrentIndex
	receivedPackets     ConcurrentReceivedPackets
	indexSinceLastRelay []int // used by funnel to determine which packets are new

	connections          map[int][]*tls.Conn  // TLS connection to funnels
	connectionsToCompute map[string]*tls.Conn // TLS connection to funnels
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
func (p *Server) Start() error {
	p.aPac = make([]node.MixPacket, 0)
	p.runningIndex.array = make([]int, threadsCount)
	p.indexSinceLastRelay = make([]int, threadsCount)
	for i := 0; i < threadsCount; i++ {
		p.runningIndex.array[i] = 0
	}

	p.receivedPackets.array = make([][][]byte, threadsCount)
	for i := 0; i < threadsCount; i++ {
		p.receivedPackets.array[i] = make([][]byte, msgCount)
	}
	p.run()
	return nil
}

func (p *Server) GetConfig() config.MixConfig {
	return p.config
}

// Function opens the listener to start listening on provider's host and port
func (p *Server) run() {
	//defer p.listener.Close()
	finish := make(chan bool)

	// wait to synchronize with other servers on round start
	sleepTime := config.GetRemainingRoundTime()
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)

	// TODO: remove if after performance testing
	if !(staticServerRole == "funnel" || staticServerRole == "compute") {
		p.setCurrentRole()
	} else {
		if staticServerRole == "funnel" {
			isMapper = true
		} else {
			isMapper = false
		}
	}

	go func() {
		// create tick
		d := time.NewTicker(config.RoundDuration)
		for {
			select {
			case <-d.C:
				time.Sleep(200)
				p.sendOutboundFunnelMessages()
				//p.setCurrentRole()
			}
		}
	}()

	/*
		go func() {
			logLocal.Infof("Server: Listening on %s", p.host+":"+p.port)
			p.listenForIncomingConnections()
		}()
	*/

	go func() {
		logLocal.Infof("Server: Starting the TLS server")
		p.startTlsServer()
	}()

	go func() {
		logLocal.Infof("Server: Preparing for relaying")
		p.relayPacket()
	}()

	<-finish
}

// Function processes the received sphinx packet, performs the
// unwrapping operation and checks whether the packet should be
// forwarded or stored. If the processing was unsuccessful and error is returned.
func (p *Server) receivedPacketWithIndex(packet []byte, someIndex int) error {
	if isMapper { //funnel functionality
		//logLocal.Info("funnel functionality")
		// p.aPac[index] = packet
		p.receivedPackets.mu.Lock()
		p.runningIndex.mu.Lock()
		p.receivedPackets.array[someIndex][p.runningIndex.array[someIndex]] = packet
		p.runningIndex.array[someIndex]++
		/*
			if p.array[someIndex] == 1 {
				logLocal.Info("First packet. Time:", time.Now())
			} else if p.array[someIndex] == msgCount {
				logLocal.Info("Last packet. Time:", time.Now())
				p.array[someIndex] = 0
			}
		*/
		logLocal.Info("array: ", p.runningIndex)
		p.receivedPackets.mu.Unlock()
		p.runningIndex.mu.Unlock()
	} else { //compute node functionality
		logLocal.Info("compute functionality")
		funnelId := p.establishConnectionToRandomFunnel()

		//newPacket, err := p.ProcessPacketInSameThread(packet)
		//if err != nil {
		//return err
		//}
		var compPacket config.ComputePacket
		proto.Unmarshal(packet, &compPacket)
		logLocal.Info("compPacket - Bytes: ", compPacket.Data)

		//logLocal.Info("NewPacket - Adress: ", newPacket.Adr.Address)
		logLocal.Info("NewPacket - Adress: ", compPacket.NextHop)

		computePacket := config.ComputePacket{Data: compPacket.Data, NextHop: compPacket.NextHop}
		// forward to random active funnel node
		//if newPacket.Flag == "\xF1" {
		p.forwardPacketToFunnel(computePacket, funnelId)
		logLocal.Info("ComputePacket - Adress: ", computePacket.NextHop)
		relayedPackets++
		//} else {
		//	logLocal.Info("Server: Packet has non-forward flag. Packet dropped")
		//}
		logLocal.Info("Relayed packets: ", relayedPackets)
		logLocal.Info("-------------------------------------------------------------")
	}
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
func (p *Server) receivedPacket(packet []byte) error {
	// logLocal.Info("Server: Received new sphinx packet")

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
	// // logLocal.Info("Server: Processed the sphinx packet at round : ", config.GetRound() )
	// // logLocal.Info("Server: Current clock time : ", (time.Now()).String())
	// // handledPackets = handledPackets +1
	// // logLocal.Info("Server: Total packets handled = ", handledPackets)
	return nil
}

func (p *Server) forwardPacket(sphinxPacket []byte, address string) error {
	packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	if err != nil {
		return err
	}

	err = p.send(packetBytes, address)
	if err != nil {
		return err
	}
	// logLocal.Info("Server: Forwarded sphinx packet")
	return nil
}

// forwardPacketTLS() opens a new TLS connection with the given address (IP/name + port) and sends the passed SphinxPacket
func (p *Server) forwardPacketTLS(sphinxPacket []byte, address string) error {
	//packetBytes, err := config.WrapWithFlag(commFlag, sphinxPacket)
	//if err != nil {
	//	return err
	///}

	cert, err := tls.LoadX509KeyPair("/home/olaf/certs/client.pem", "/home/olaf/certs/client.key")
	if err != nil {
		logLocal.Info("compute node: loadkeys: ", err)
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true, MinVersion: 2}
	conn, err := tls.Dial("tcp", address, &config)
	if err != nil {
		logLocal.Error("Couldn't create TLS connection with peer.", address)
		logLocal.Error(err)
		return err
	}
	conn.Write(sphinxPacket)
	if err != nil {
		return err
	}
	return nil
}

func (p *Server) forwardPacketToFunnel(computePacket config.ComputePacket, funnelId int) error {
	computeBytes, err := proto.Marshal(&computePacket)
	if err != nil {
		logLocal.WithError(err)
	}
	packetBytes, err := config.WrapWithFlag(commFlag, computeBytes)
	if err != nil {
		return err
	}

	randPort := mrand.Int31n(int32(threadsCount - 1))
	p.connections[funnelId][randPort].Write(packetBytes)
	if err != nil {
		return err
	}
	logLocal.Info("Server: Forwarded sphinx packet to funnel with id and port: ", funnelId, randPort)
	return nil
}

// Function opens a connection with selected network address
// and send the passed packet. If connection failed or
// the packet could not be send, an error is returned
func (p *Server) send(packet []byte, address string) error {

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
func (p *Server) listenForIncomingConnections() { //deprecated
	var wg sync.WaitGroup
	for i := 0; i < 180; i++ {
		wg.Add(1)
		// conn, err := p.listener.Accept()
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
	logLocal.Info("Server: packets processing done at round : ", config.GetRound())
	// logLocal.Info("Server: Total packets processed : ", handledPackets )
}

func (p *Server) relayPacket() error {
	for {
		// delayBeforeContinute(config.RoundDuration, config.SyncTime)
		// packets := make([]node.MixPacket, len(p.aPac))
		// p.mutex.Lock()
		// copy(packets, p.aPac)
		// p.mutex.Unlock()
		// // p.mutex.Lock()
		// shuffle()
		for _, packet := range p.aPac {
			if packet.Flag == "\xF1" {
				p.forwardPacket(packet.Data, packet.Adr.Address)
				// packetsRelayed = packetsRelayed +1
				// logLocal.Info("MixServer: Total number of packets relayed", packetsRelayed)
			} else {
				logLocal.Info("MixServer: Packet has non-forward flag. Packet dropped")
			}
		}
		p.mutex.Lock()
		p.aPac = p.aPac[0:0]
		p.mutex.Unlock()
	}
}

func (p *Server) relayPacketAsFunnel(packetBytes []byte) {
	// unmarshal into general packet
	var generalPacket config.GeneralPacket
	err := proto.Unmarshal(packetBytes, &generalPacket)
	if err != nil {
		logLocal.WithError(err)
	}

	var computePacket config.ComputePacket
	err = proto.Unmarshal(generalPacket.Data, &computePacket)
	if err != nil {
		logLocal.WithError(err)
	}
	// experimental loadbalancing here - use different destination ports for relay
	//logLocal.Info("Address to be processed: ", computePacket.NextHop)
	if len(computePacket.NextHop) == 0 {
		//logLocal.Info("Empty next hop! Not relaying!")
		emptyCtr++
		logLocal.Error("Not relayed due to empty next hop: ", emptyCtr)
		logLocal.Info("Payload: ", computePacket.Data)
		return
	}
	dstIp := strings.SplitAfter(computePacket.NextHop, ":")[0]
	dbPort := strings.SplitAfter(computePacket.NextHop, ":")[1]

	dbPortInt, err := strconv.Atoi(dbPort)
	if err != nil {
		logLocal.Error("Couldn't read port from packet to relay!", err)
	}

	if dbPortInt < 10000 && dbPortInt >= 9900 { // hardcoded portrange for protocol for now
		dstAddr := dstIp + strconv.Itoa(dbPortInt+lbCtr)
		// for this to work, every server has to have the same amount of threads
		lbCtr = (lbCtr + 1) % threadsCount

		// save connection to map if it doesn't exist
		conn, pres := p.connectionsToCompute[dstAddr]

		if !pres {
			cert, err := tls.LoadX509KeyPair("/home/olaf/certs/client.pem", "/home/olaf/certs/client.key")
			if err != nil {
				logLocal.Info("compute node: loadkeys: ", err)
			}
			config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true, MinVersion: 2}
			conn, err := tls.Dial("tcp", dstAddr, &config)
			if err != nil {
				logLocal.Error("Couldn't create TLS connection with peer.", dstAddr)
				logLocal.Error(err)
			}
			p.connectionsToCompute[dstAddr] = conn
			conn.Write(computePacket.Data)
			if err != nil {
				logLocal.Error("Error sending packet to compute.", err)
			}
		} else {
			_, err := conn.Write(computePacket.Data)
			if err != nil {
				logLocal.Error("Error sending packet to compute.", err)
			}
		}
		//logLocal.Info("Next Hop old: ", computePacket.NextHop)
		//logLocal.Info("Next Hop new: ", dstAddr)
		//p.forwardPacketTLS(computePacket.Data, dstAddr) // data in computePacket is a SphinxPacket
	} else {
		//logLocal.Info("Next Hop: ", computePacket.NextHop)
		p.forwardPacketTLS(computePacket.Data, computePacket.NextHop) // data in computePacket is a SphinxPacket
	}

}

// startTlsServer() opens multiple TLS listeners on multiple ports starting with the port given during server start.
// The amount pf listeners depends on thread count.
func (p *Server) startTlsServer() error {
	cert, err := tls.LoadX509KeyPair("/home/olaf/certs/server.pem", "/home/olaf/certs/server.key")
	if err != nil {
		// log.Fatalf("server: loadkeys: %s", err)
		logLocal.Info("server: loadkeys: ", err)
		return err
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}}

	// someIndex := 0
	ip, err := helpers.GetLocalIP()
	if err != nil {
		panic(err)
	}
	for someIndex := 0; someIndex < threadsCount; someIndex++ {
		config.Rand = rand.Reader
		intPort, _ := strconv.Atoi(p.port)
		port := intPort + someIndex
		//service := "127.0.0.1:" + strconv.Itoa(port)
		service := ip + ":" + strconv.Itoa(port)
		listener, err := tls.Listen("tcp", service, &config)
		if err != nil {
			// log.Fatalf("server: listen: %s", err)
			logLocal.Info("server: listen: ", err)
			return err
		}
		logLocal.Info("server: listening on port ", port)

		go func(localIndex int) {
			for {
				conn, err := listener.Accept()
				if err != nil {
					logLocal.Info("server: accept: ", err)
					break
				}
				// defer conn.Close()
				logLocal.Info("server: accepted from ", conn.RemoteAddr())
				tlscon, ok := conn.(*tls.Conn)
				if ok {
					logLocal.Info("ok=true")
					state := tlscon.ConnectionState()
					for _, v := range state.PeerCertificates {
						logLocal.Info(x509.MarshalPKIXPublicKey(v.PublicKey))
					}
				}
				go p.handleClient(conn, localIndex)
				// someIndex++
			}
		}(someIndex) // someIndex as input for localIndex
	}
	return nil
}

func (p *Server) handleClient(conn net.Conn, someIndex int) {
	// defer conn.Close()
	buf := make([]byte, 1024)
	for {
		// logLocal.Info("server: conn: waiting")
		n, err := conn.Read(buf)
		if err != nil {
			logLocal.Info("server: conn: read: ", err)
			break
		}

		// logLocal.Info("server: conn: echo %q\n", string(buf[:n]))
		// n, err = conn.Write(buf[:n])
		// n, err = conn.Write(buf[:n])
		// logLocal.Info("server: conn: wrote %d bytes", n)
		// if err != nil {
		//     logLocal.Info("server: write: %s", err)
		//     break
		// }

		// var packet SphinxPacket
		// err = proto.Unmarshal(buf[:n], &packet)
		// if err != nil {
		// 	logLocal.WithError(err).Error(err)
		// 	// return err
		// }
		p.receivedPacketWithIndex(buf[:n], someIndex)

		// switch string(packet.Flag) {
		// // case string(assignFlag):
		// // 	err = p.handleAssignRequest(packet.Data)
		// // 	if err != nil {
		// // 		logLocal.WithError(err).Error(err)
		// // 		return err
		// // 	}
		// case string(commFlag):
		// 	err = p.receivedPacket(packet.Data)
		// 	if err != nil {
		// 		logLocal.WithError(err).Error(err)
		// 		return err
		// 	}
		// // case string(pullFlag):
		// // 	err = p.handlePullRequest(packet.Data)
		// // 	if err != nil {
		// // 		logLocal.WithError(err).Error(err)
		// // 		return err
		// // 	}
		// default:
		// 	logLocal.Info(packet.Flag)
		// 	logLocal.Info("Server: Packet flag not recognised. Packet dropped")
		// 	return nil
		// }
	}
	logLocal.Info("server: conn: closed")
}

/* // not required now...
func (p *Server) createTlsConnection() {
    cert, err := tls.LoadX509KeyPair("certs/client.pem", "certs/client.key")
    if err != nil {
        logLocal.Info("server: loadkeys: ", err)
    }
    config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
    conn, err := tls.Dial("tcp", "127.0.0.1:8000", &config)
    if err != nil {
        logLocal.Info("client: dial: ", err)
    }
    defer conn.Close()
    logLocal.Info("client: connected to: ", conn.RemoteAddr())

    state := conn.ConnectionState()
    for _, v := range state.PeerCertificates {
        fmt.Println(x509.MarshalPKIXPublicKey(v.PublicKey))
        fmt.Println(v.Subject)
    }
    logLocal.Info("client: handshake: ", state.HandshakeComplete)
    logLocal.Info("client: mutual: ", state.NegotiatedProtocolIsMutual)

    message := "Hello\n"
    _, err = io.WriteString(conn, message)
    if err != nil {
        logLocal.Info("client: write: ", err)
    }
    // logLocal.Info("client: wrote %q (%d bytes)", message, n)

    reply := make([]byte, 256)
    _, err = conn.Read(reply)
    // logLocal.Info("client: read %q (%d bytes)", string(reply[:n]), n)
    logLocal.Info("client: exiting")
} */

// HandleConnection handles the received packets; it checks the flag of the
// packet and schedules a corresponding process function and returns an error.
func (p *Server) handleConnection(conn net.Conn) error {

	buff := make([]byte, 1024)
	reqLen, err := conn.Read(buff)
	// defer conn.Close()

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
	// case string(assignFlag):
	// 	err = p.handleAssignRequest(packet.Data)
	// 	if err != nil {
	// 		logLocal.WithError(err).Error(err)
	// 		return err
	// 	}
	case string(commFlag):
		err = p.receivedPacket(packet.Data)
		if err != nil {
			logLocal.WithError(err).Error(err)
			return err
		}
	// case string(pullFlag):
	// 	err = p.handlePullRequest(packet.Data)
	// 	if err != nil {
	// 		logLocal.WithError(err).Error(err)
	// 		return err
	// 	}
	default:
		logLocal.Info(packet.Flag)
		logLocal.Info("Server: Packet flag not recognised. Packet dropped")
		return nil
	}
	return nil
}

/*
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
} */

/* // AuthenticateUser compares the authentication token received from the client with
// the one stored by the provider. If tokens are the same, it returns true
// and false otherwise.
func (p *Server) authenticateUser(clientId string, clientToken []byte) bool {

	if bytes.Compare(p.assignedClients[clientId].token, clientToken) == 0 {
		return true
	}
	logLocal.Warningf("ProviderServer: Non matching token: %s, %s", p.assignedClients[clientId].token, clientToken)
	return false
} */

// NewServer constructs a new server object.
// NewServer returns a new server object and an error.
func NewServer(id string, host string, port string, pubKey []byte, prvKey []byte, pkiPath string, staticRole string) (*Server, error) {
	staticServerRole = staticRole
	node := node.NewMix(pubKey, prvKey)
	server := Server{id: id, host: host, port: port, Mix: node, listener: nil}
	server.config = config.MixConfig{Id: server.id, Host: server.host, Port: server.port, PubKey: server.GetPublicKey()}
	server.assignedClients = make(map[string]ClientRecord)
	server.connections = make(map[int][]*tls.Conn)
	server.connectionsToCompute = make(map[string]*tls.Conn)

	threadsCount = runtime.NumCPU()
	//logLocal.Info("Starting server with logical cores: ", threadsCount)

	// prevent server from adding its config multiple times to database
	logLocal.Info("Deleting old DB entry...")
	err := helpers.RemoveDBEntryForServerId(pkiPath, "Pki", server.id)
	if err != nil {
		logLocal.Error("Error deleting old DB entry for node.", err)
	}
	logLocal.Info("Finished deleting old DB entry.")
	configBytes, err := proto.Marshal(&server.config)
	if err != nil {
		return nil, err
	}
	err = helpers.AddToDatabase(pkiPath, "Pki", server.id, "Provider", configBytes)
	if err != nil {
		return nil, err
	}

	return &server, nil
}

// establishConnectionToRandomFunnel() establishes a new persistent connection to one of the available funnel nodes.
// The function chooses random funnel to connect to. If the connection to that node is already established, no new connection is created.
// Returns the funnelId of the node it connected to.
func (p *Server) establishConnectionToRandomFunnel() int {
	// get current funnels
	// TODO: re-enable after performance testing, DIRTY HACK!!
	//list := helpers.GetCurrentFunnelNodes(globalNodeCount)
	//randNumber := mrand.Int31n(int32(len(list) - 1)) // 1=funnelCount-1
	//funnelId := list[randNumber]

	// --- THIS HAS TO BE REMOVED AFTERWARDS ---
	/*
		funnelId := 0
		if config.GetRound()%2 == 0 {
			funnelId = 2
		} else {
			funnelId = 3
		}

	*/
	// --- THIS HAS TO BE REMOVED AFTERWARDS ---

	// use this for testing with only one funnel
	funnelId := 2

	// --- THIS HAS TO BE REMOVED AFTERWARDS ---

	logLocal.Info("funnelId: ", funnelId)
	// check if there already exists a connection to that funnel
	_, pres := p.connections[funnelId]
	if !pres {
		// check database for nodes which act as funnels
		db, err := pki.OpenDatabase(PKI_DIR, "sqlite3")
		if err != nil {
			panic(err)
		}
		row := db.QueryRow("SELECT Config FROM Pki WHERE Id = ? AND Typ = ?", strconv.Itoa(funnelId), "Provider")

		var results []byte
		err = row.Scan(&results)
		if err != nil {
			fmt.Println(err)
		}

		var mixConfig config.MixConfig
		err = proto.Unmarshal(results, &mixConfig)

		// establish a connection with them using all available ports stated by threadsCount
		cert, err := tls.LoadX509KeyPair("/home/olaf/certs/client.pem", "/home/olaf/certs/client.key")
		if err != nil {
			logLocal.Info("compute node: loadkeys: ", err)
		}
		config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true, MinVersion: 2}

		nodeHost := mixConfig.Host
		intPort, _ := strconv.Atoi(mixConfig.Port)
		p.connections[funnelId] = make([]*tls.Conn, threadsCount)

		// open connection with every available port for multithreading the ingress of funnel nodes
		i := 0
		for i < threadsCount {
			realPort := intPort + i
			nodePort := strconv.Itoa(realPort)
			conn, err := tls.Dial("tcp", nodeHost+":"+nodePort, &config)
			if err != nil {
				logLocal.Info("compute node: dial: ", err)
			}
			p.connections[funnelId][i] = conn
			logLocal.Info("compute node: connected to funnel: ", conn.RemoteAddr())
			i = i + 1
		}
		// state := conn.ConnectionState() Debug Info about connection
	}
	return funnelId
}

// rearrangeReceivedPackets transfers all packets from the 3D array receivedPackets to a new outbound array and shuffles them.
func (p *Server) rearrangeReceivedPackets() [][]byte {
	outboundPackets := make([][]byte, 0)
	// iterate over the packages of each thread but just until the index marking new packages ends so nothing is sent multiple times
	p.receivedPackets.mu.Lock()
	p.runningIndex.mu.Lock()
	for i, packagesPerThread := range p.receivedPackets.array { // first dimension is per thread
		workIndex := p.runningIndex.array[i]
		newPackages := packagesPerThread[p.indexSinceLastRelay[i]:workIndex]
		p.indexSinceLastRelay[i] = workIndex
		for _, packet := range newPackages {
			if len(packet) > 0 {
				outboundPackets = append(outboundPackets, packet)
			}
		}
	}
	p.receivedPackets.mu.Unlock()
	p.runningIndex.mu.Unlock()
	mrand.Seed(time.Now().Unix())
	mrand.Shuffle(len(outboundPackets), func(i, j int) {
		outboundPackets[i], outboundPackets[j] = outboundPackets[j], outboundPackets[i]
	})
	// reset indices to use in next round
	/*
		for i := 0; i < threadsCount; i++ {
			p.array[i] = 0
		}
	*/
	return outboundPackets
}

// setCurrentRole() sets the nodewide status determining how the node behaves when accepting and relaying packets.
func (p *Server) setCurrentRole() {
	// get current funnels and compare with own id and set server flag
	listOfFunnels := helpers.GetCurrentFunnelNodes(globalNodeCount)
	// isMapper false --> compute, reset flag
	isMapper = false
	for _, funnelId := range listOfFunnels {
		// isMapper true --> funnel
		serverId, _ := strconv.Atoi(p.id) // this just works with IDs that dont contain ASCII characters
		if funnelId == int(serverId) {
			isMapper = true
		}
	}
}

// sendOutboundFunnelMessages() is a funnel function.
// It sends out collected packets to their next hop without removing a layer of crypto.
// Before sending, the function checks if the node is actually a funnel node and if there are packets to send.
func (p *Server) sendOutboundFunnelMessages() {
	// reduce dimension of outbound packet array
	outboundPackets := p.rearrangeReceivedPackets()
	//logLocal.Info("Outbound packets: ", len(outboundPackets))
	if len(outboundPackets) > 0 {
		// relay packets here if funnel
		if isMapper {
			for _, packet := range outboundPackets {
				p.relayPacketAsFunnel(packet)
				relayedPackets++
			}
			logLocal.Info("Sent all outbound packages as funnel.")
			logLocal.Info("Relayed packets: ", relayedPackets)
		}
	}
}
