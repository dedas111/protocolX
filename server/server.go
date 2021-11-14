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
	// runningIndex = 0
	msgCount     = 100000
	threadsCount = 100

	logLocal = logging.PackageLogger()
)

const (
	PKI_DIR = "pki/database.db"
)

type ProviderIt interface {
	networker.NetworkServer
	networker.NetworkClient
	Start() error
	GetConfig() config.MixConfig
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

	aPac            []node.MixPacket
	receivedPackets [][][]byte
	mutex           sync.Mutex
	runningIndex    []int

	// TODO make dynamic with map id/connection
	connections map[string]*tls.Conn // TLS connection to funnels
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
	p.runningIndex = make([]int, threadsCount)
	for i := 0; i < threadsCount; i++ {
		p.runningIndex[i] = 0
	}

	p.receivedPackets = make([][][]byte, threadsCount)
	for i := 0; i < threadsCount; i++ {
		p.receivedPackets[i] = make([][]byte, msgCount)
	}
	p.run()
	return nil
}

func (p *Server) GetConfig() config.MixConfig {
	return p.config
}

// Function opens the listener to start listening on provider's host and port
func (p *Server) run() {

	defer p.listener.Close()
	finish := make(chan bool)

	// wait to synchronize with other servers on round start
	sleepTime := config.GetRemainingRoundTime()
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)

	go func() {
		// create tick
		d := time.NewTicker(config.RoundDuration)
		for {
			select {
			case <-d.C:
				p.sendOutboundFunnelMessages()
				p.setCurrentRole()
			}
		}
	}()

	go func() {
		logLocal.Infof("Server: Listening on %s", p.host+":"+p.port)
		p.listenForIncomingConnections()
	}()

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
		// p.aPac[index] = packet
		p.receivedPackets[someIndex][p.runningIndex[someIndex]] = packet
		p.runningIndex[someIndex]++
		if p.runningIndex[someIndex] == 1 {
			logLocal.Info("First packet. Time:", time.Now())
		} else if p.runningIndex[someIndex] == msgCount {
			logLocal.Info("Last packet. Time:", time.Now())
			p.runningIndex[someIndex] = 0
		}
	} else { //compute node functionality
		newPacket, err := p.ProcessPacketInSameThread(packet)
		if err != nil {
			return err
		}
		if newPacket.Flag == "\xF1" {
			p.forwardPacket(newPacket.Data, newPacket.Adr.Address)
			// packetsRelayed = packetsRelayed +1
			// logLocal.Info("MixServer: Total number of packets relayed", packetsRelayed)
		} else {
			logLocal.Info("Server: Packet has non-forward flag. Packet dropped")
		}
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
	newPacket, err := p.ProcessPacketForRelayInFunnel(packetBytes)
	if err != nil {
		logLocal.Info("MixServer: Packet couldn't be preprocessed for relay from funnel. Packet dropped")
		return
	}
	p.forwardPacket(newPacket.Data, newPacket.Adr.Address)
}

func (p *Server) startTlsServer() error {
	cert, err := tls.LoadX509KeyPair("/home/olaf/certs/server.pem", "/home/olaf/certs/server.key")
	if err != nil {
		// log.Fatalf("server: loadkeys: %s", err)
		logLocal.Info("server: loadkeys: ", err)
		return err
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}}

	// someIndex := 0
	for someIndex := 0; someIndex < threadsCount; someIndex++ {
		config.Rand = rand.Reader
		port := 9960 + someIndex
		service := "127.0.0.1:" + strconv.Itoa(port)
		listener, err := tls.Listen("tcp", service, &config)
		if err != nil {
			// log.Fatalf("server: listen: %s", err)
			logLocal.Info("server: listen: ", err)
			return err
		}
		logLocal.Info("server: listening")

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
		}(someIndex)
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

	// addr, err := helpers.ResolveTCPAddress(server.host, server.port)
	addr := server.host + ":" + server.port
	if err != nil {
		return nil, err
	}
	//server.listener, err = net.ListenTCP("tcp", addr)
	cert, err := tls.LoadX509KeyPair("/home/olaf/certs/server.pem", "/home/olaf/certs/server.key")
	if err != nil {
		// log.Fatalf("server: loadkeys: %s", err)
		logLocal.Info("server: loadkeys: ", err)
		panic(err)
	}
	conf := tls.Config{Certificates: []tls.Certificate{cert}}
	server.listener, err = tls.Listen("tcp", addr, &conf)

	if err != nil {
		return nil, err
	}

	return &server, nil
}

// Establish a new persistent connection to one of the available funnel nodes
// choose random funnel
func (p *Server) establishConnectionToRandomFunnel() {
	// get current funnels
	list := helpers.GetCurrentFunnelNodes(5)
	randNumber := mrand.Int31n(1) // 1=funnelCount-1
	funnelId := list[randNumber]
	// check if there already exists a connection to that funnel
	_, pres := p.connections[strconv.Itoa(funnelId)]
	if !pres {
		// check database for nodes which act as funnels
		db, err := pki.OpenDatabase(PKI_DIR, "sqlite3")
		if err != nil {
			panic(err)
		}
		row := db.QueryRow("SELECT Config FROM Pki WHERE Id = ? AND Typ = ?", "Provider"+strconv.Itoa(funnelId), "provider")

		var results []byte
		err = row.Scan(&results)
		if err != nil {
			fmt.Println(err)
		}

		var mixConfig config.MixConfig
		err = proto.Unmarshal(results, &mixConfig)

		nodeHost := mixConfig.Host
		nodePort := mixConfig.Port

		// establish a connection with them
		cert, err := tls.LoadX509KeyPair("certs/client.pem", "certs/client.key")
		if err != nil {
			logLocal.Info("compute node: loadkeys: ", err)
		}
		config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
		conn, err := tls.Dial("tcp", nodeHost+":"+nodePort, &config)
		if err != nil {
			logLocal.Info("compute node: dial: ", err)
		}
		logLocal.Info("compute node: connected to: ", conn.RemoteAddr())
		// state := conn.ConnectionState() Debug Info about connection
		// add connection to map if
		p.connections[strconv.Itoa(funnelId)] = conn
	}
}

// rearrangeReceivedPackets transfers all packets from the 3D array receivedPackets to a new outbound array and shuffles them
func (p *Server) rearrangeReceivedPackets() [][]byte {
	outboundPackets := make([][]byte, 0)
	// iterate over the packages of each thread but just until the index marking new packages ends so nothing is sent multiple times
	for i, packagesPerThread := range p.receivedPackets {
		numOfPacketsInThread := p.runningIndex[i]
		newPackages := packagesPerThread[:numOfPacketsInThread]
		for _, packet := range newPackages {
			outboundPackets = append(outboundPackets, packet)
		}
	}
	mrand.Seed(time.Now().Unix())
	mrand.Shuffle(len(outboundPackets), func(i, j int) {
		outboundPackets[i], outboundPackets[j] = outboundPackets[j], outboundPackets[i]
	})
	// reset indices to use in next round
	for i := 0; i < threadsCount; i++ {
		p.runningIndex[i] = 0
	}
	return outboundPackets
}

func (p *Server) setCurrentRole() {
	// get current funnels and compare with own id and set server flag
	listOfFunnels := helpers.GetCurrentFunnelNodes(2)
	// isMapper false --> compute, reset flag
	isMapper = false
	for _, funnelId := range listOfFunnels {
		// isMapper true --> funnel
		serverId, _ := strconv.Atoi(p.id)
		if funnelId == int(serverId) {
			isMapper = true
		}
	}
}

func (p *Server) sendOutboundFunnelMessages() {
	if len(p.receivedPackets) > 0 {
		// reduce dimension of outbound packet array
		outboundPackets := p.rearrangeReceivedPackets()
		// relay packets here if funnel
		if isMapper {
			for _, packet := range outboundPackets {
				p.relayPacketAsFunnel(packet)
			}
		}
	}
}
