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
	helpers "github.com/dedas111/protocolX/helpers"
	"github.com/dedas111/protocolX/node"
	"github.com/dedas111/protocolX/pki"
	"github.com/dedas111/protocolX/sphinx"
	mrand "math/rand"

	//"crypto/curve25519"
	// "crypto/aes"
	// "crypto/cipher"
	"crypto/elliptic"
	// "crypto/rand"
	"crypto/tls"
	"crypto/x509"

	"github.com/golang/protobuf/proto"
	// "github.com/stretchr/testify/assert"

	// "errors"
	"fmt"
	// "math/big"
	"sync"
	"time"
	// "io"
	// "io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

var remoteServer *Server
var localServer *Server
var anotherServer *Server
var curve = elliptic.P224()

const (
	testDatabase = "testDatabase.db"
)

func createTestServer() (*Server, error) {
	pub, priv, err := sphinx.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	n := node.NewMix(pub, priv)
	provider := Server{host: "localhost", port: "9999", Mix: n}
	provider.config = config.MixConfig{Id: provider.id, Host: provider.host, Port: provider.port, PubKey: provider.GetPublicKey()}
	provider.assignedClients = make(map[string]ClientRecord)
	provider.aPac = make([]node.MixPacket, 0)
	return &provider, nil
}

// func createTestProviderWithPort(serverPort string) (*Server, error) {
// 	pub, priv, err := sphinx.GenerateKeyPair()
// 	if err != nil {
// 		return nil, err
// 	}
// 	n := node.NewMix(pub, priv)
// 	provider := Server{host: "localhost", port: serverPort, Mix: n}
// 	provider.config = config.MixConfig{Id: provider.id, Host: provider.host, Port: provider.port, PubKey: provider.GetPublicKey()}
// 	provider.assignedClients = make(map[string]ClientRecord)
// 	provider.aPac = make([]node.MixPacket, 0)

// 	addr, err := helpers.ResolveTCPAddress(mix.host, mix.port)
// 	if err != nil {
// 		return nil, err
// 	}
// 	provider.listener, err = net.ListenTCP("tcp", addr)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &provider, nil
// }

func createTestRemoteServer() (*Server, error) {
	pub, priv, err := sphinx.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	n := node.NewMix(pub, priv)
	mix := Server{host: "localhost", port: "9995", Mix: n}
	mix.config = config.MixConfig{Id: mix.id, Host: mix.host, Port: mix.port, PubKey: mix.GetPublicKey()}
	mix.aPac = make([]node.MixPacket, 0)

	addr, err := helpers.ResolveTCPAddress(mix.host, mix.port)
	if err != nil {
		return nil, err
	}
	mix.listener, err = net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &mix, nil
}

func createFakeClientListener(host, port string) (*net.TCPListener, error) {
	addr, err := helpers.ResolveTCPAddress(host, port)
	if err != nil {
		return nil, err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return listener, nil
}

func clean() {
	os.RemoveAll("./inboxes")
}

func TestMain(m *testing.M) {
	// curve = elliptic.P224()
	var err error
	fmt.Println("created nothing.")
	remoteServer, err = createTestServer()
	if err != nil {
		fmt.Println(err)
		panic(m)
	}
	fmt.Println("created remote server.")

	localServer, err = createTestServer()
	if err != nil {
		fmt.Println(err)
		panic(m)
	}
	fmt.Println("created local server.")

	// anotherServer, err = createTestProviderWithPort("9998")
	// if err != nil {
	// 	fmt.Println(err)
	// 	panic(m)
	// }

	code := m.Run()
	clean()
	os.Exit(code)
}

// func TestServer_AuthenticateUser_Pass(t *testing.T) {
// 	testToken := []byte("AuthenticationToken")
// 	record := ClientRecord{id: "Alice", host: "localhost", port: "1111", pubKey: nil, token: testToken}
// 	providerServer.assignedClients["Alice"] = record
// 	assert.True(t, providerServer.authenticateUser("Alice", []byte("AuthenticationToken")), " Authentication should be successful")
// }

// func TestServer_AuthenticateUser_Fail(t *testing.T) {
// 	record := ClientRecord{id: "Alice", host: "localhost", port: "1111", pubKey: nil, token: []byte("AuthenticationToken")}
// 	providerServer.assignedClients["Alice"] = record
// 	assert.False(t, providerServer.authenticateUser("Alice", []byte("WrongAuthToken")), " Authentication should not be successful")
// }

func createInbox(id string, t *testing.T) {
	path := filepath.Join("./inboxes", id)
	exists, err := helpers.DirExists(path)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		os.RemoveAll(path)
		os.MkdirAll(path, 0755)
	} else {
		os.MkdirAll(path, 0755)
	}
}

func createTestMessage(id string, t *testing.T) {

	file, err := os.Create(filepath.Join("./inboxes", id, "TestMessage.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	_, err = file.Write([]byte("This is a test message"))
	if err != nil {
		t.Fatal(err)
	}

}

/*
// We do not need the following tests, because we do not have providers anymore.

func TestProviderServer_FetchMessages_FullInbox(t *testing.T) {
	clientListener, err := createFakeClientListener("localhost", "9999")
	defer clientListener.Close()

	providerServer.assignedClients["FakeClient"] = ClientRecord{"FakeClient",
		"localhost",
		"9999",
		[]byte("FakePublicKey"),
		[]byte("TestToken")}

	createInbox("FakeClient", t)
	createTestMessage("FakeClient", t)

	signal, err := providerServer.fetchMessages("FakeClient")
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "SI", signal, " For inbox containing messages the signal should be SI")
}

func TestProviderServer_FetchMessages_EmptyInbox(t *testing.T) {
	createInbox("EmptyInbox", t)
	signal, err := providerServer.fetchMessages("EmptyInbox")
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "EI", signal, " For an empty inbox id the function should return signal EI")
}

func TestProviderServer_FetchMessages_NoInbox(t *testing.T) {
	signal, err := providerServer.fetchMessages("NonExistingInbox")
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "NI", signal, " For a non-existing inbox id the function should return signal NI")
}

func TestProviderServer_StoreMessage(t *testing.T) {

	inboxId := "ClientInbox"
	fileId := "12345"
	inboxDir := "./inboxes/" + inboxId
	filePath := inboxDir + "/" + fileId + ".txt"

	err := os.MkdirAll(inboxDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("Hello world message")
	providerServer.storeMessage(message, inboxId, fileId)

	_, err = os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	assert.Nil(t, err, "The file with the message should be created")

	dat, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, message, dat, "Messages should be the same")

}

func TestProviderServer_HandlePullRequest_Pass(t *testing.T) {
	testPullRequest := config.PullRequest{ClientId: "PassTestId", Token: []byte("TestToken")}
	providerServer.assignedClients["PassTestId"] = ClientRecord{id: "TestId", host: "localhost", port: "1111", pubKey: nil, token: []byte("TestToken")}
	bTestPullRequest, err := proto.Marshal(&testPullRequest)
	if err != nil {
		t.Error(err)
	}
	err = providerServer.handlePullRequest(bTestPullRequest)
	if err != nil {
		t.Error(err)
	}
}

func TestProviderServer_HandlePullRequest_Fail(t *testing.T) {
	testPullRequest := config.PullRequest{ClientId: "FailTestId", Token: []byte("TestToken")}
	providerServer.assignedClients = map[string]ClientRecord{}
	bTestPullRequest, err := proto.Marshal(&testPullRequest)
	if err != nil {
		t.Error(err)
	}
	err = providerServer.handlePullRequest(bTestPullRequest)
	assert.EqualError(t, errors.New("Server: authentication went wrong"), err.Error(), "HandlePullRequest should return an error if authentication failed")
}

func TestProviderServer_RegisterNewClient(t *testing.T) {
	newClient := config.ClientConfig{Id: "NewClient", Host: "localhost", Port: "9998", PubKey: nil}
	bNewClient, err := proto.Marshal(&newClient)
	if err != nil {
		t.Fatal(err)
	}
	token, addr, err := providerServer.registerNewClient(bNewClient)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "localhost:9998", addr, "Returned address should be the same as registered client address")
	assert.Equal(t, helpers.SHA256([]byte("TMP_Token"+"NewClient")), token, "Returned token should be equal to the hash of clients id")

	path := fmt.Sprintf("./inboxes/%s", "NewClient")
	exists, err := helpers.DirExists(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, exists, "When a new client is registered an inbox should be created")
}

func TestProviderServer_HandleAssignRequest(t *testing.T) {
	clientListener, err := createFakeClientListener("localhost", "9999")
	defer clientListener.Close()

	newClient := config.ClientConfig{Id: "ClientXYZ", Host: "localhost", Port: "9999", PubKey: nil}
	bNewClient, err := proto.Marshal(&newClient)
	if err != nil {
		t.Fatal(err)
	}
	err = providerServer.handleAssignRequest(bNewClient)
	if err != nil {
		t.Fatal(err)
	}
}
*/

func createTlsConnection(port int, t *testing.T) net.Conn {
	t.Log("Before client loadkeys")
	cert, err := tls.LoadX509KeyPair("/home/olaf/certs/client.pem", "/home/olaf/certs/client.key")
	if err != nil {
		t.Log("server: loadkeys")
		return nil
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port), &config)
	if conn == nil {
		t.Log("Conn is nil")
		// retunr nil
	}
	// defer conn.Close()
	t.Log("client: connected to: ", conn.RemoteAddr())

	state := conn.ConnectionState()
	for _, v := range state.PeerCertificates {
		fmt.Println(x509.MarshalPKIXPublicKey(v.PublicKey))
		fmt.Println(v.Subject)
	}
	t.Log("client: handshake: ", state.HandshakeComplete)
	t.Log("client: mutual: ", state.NegotiatedProtocolIsMutual)

	// message := "Hello\n"
	// n, err := io.WriteString(conn, message)
	// if err != nil {
	//     logLocal.Info("client: write: %s", err)
	// }
	// logLocal.Info("client: wrote %q (%d bytes)", message, n)
	return conn
}

func TestServer_TlsConnectionReceive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// t.Log("Before the server starts")
	// time.Sleep(300 * time.Millisecond)
	// localServer.startTlsServer()

	threadsCount = 2
	var connections = make([]net.Conn, threadsCount)

	for i := 0; i < threadsCount; i++ {
		t.Log("After the server starts")
		// time.Sleep(20 * time.Millisecond)
		port := 9960 + i
		connections[i] = createTlsConnection(port, t)
		if connections[i] == nil {
			t.Log("Conn is nil")
			// retunr nil
		}

		t.Log("After the TLS connection is established")
		time.Sleep(30 * time.Millisecond)
	}

	totalPackets := 5
	sphinxPacket := createTestPacket(t, "hello world")
	// sphinxPacket := createLargeTestPacket(t, "hello world")
	bSphinxPacket, err := proto.Marshal(sphinxPacket)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Timestamp before the testrun starts : ", time.Now())

	var waitgroup sync.WaitGroup
	for j := 0; j < threadsCount; j++ {
		waitgroup.Add(1)
		conn := connections[j]
		go func(connection net.Conn) {
			defer waitgroup.Done()
			for i := 0; i < totalPackets; i++ {
				_, err := connection.Write(bSphinxPacket)
				if err != nil {
					t.Log("There is an error : ", err)
				}
				// else {
				// 	t.Log("Packet sent with bytes : ", n)
				// }
			}
		}(conn)
	}
	waitgroup.Wait()
	t.Log("Timestamp after the testrun ends : ", time.Now())
	// t.Log("After the TLS connection is established")
	// time.Sleep(300 * time.Millisecond)
	// assert.Equal(t, totalPackets, localServer.runningIndex, "All the messages are not processed.")
}

func TestServer_TlsMemoryLoad(t *testing.T) {
	// if testing.Short() {
	//     t.Skip("skipping test in short mode.")
	// }

	// t.Log("Before the server starts")
	// time.Sleep(300 * time.Millisecond)
	// localServer.startTlsServer()

	threadsCount = 100
	var connections = make([]net.Conn, threadsCount)

	for i := 0; i < threadsCount; i++ {
		t.Log("After the server starts")
		// time.Sleep(20 * time.Millisecond)
		port := 9960 + i
		connections[i] = createTlsConnection(port, t)
		if connections[i] == nil {
			t.Log("Conn is nil")
			// retunr nil
		}

		t.Log("After the TLS connection is established")
		time.Sleep(30 * time.Millisecond)
	}

	// totalPackets := 12500 * 4
	totalPackets := 1000000
	sentPackets := make([][]byte, totalPackets)
	PrintMemUsage()
	t.Log("Timestamp before the testrun starts : ", time.Now())

	sphinxPacket := createLargeTestPacket(t, fmt.Sprintf("%d : hello world", 343))
	bSphinxPacket, err := proto.Marshal(sphinxPacket)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("The size of one packet in bytes: ", len(bSphinxPacket))

	for i := 0; i < totalPackets; i++ {
		sentPackets[i] = make([]byte, len(bSphinxPacket))
		copy(sentPackets[i], bSphinxPacket)
	}

	// var waitgroup sync.WaitGroup
	// for j := 0; j < threadsCount; j++ {
	// 	waitgroup.Add(1)
	// 	conn := connections[j]
	//     go func(connection net.Conn) {
	// 		defer waitgroup.Done()
	//         for i := 0; i < totalPackets; i++ {
	// 			_, err := connection.Write(bSphinxPacket)
	// 			if err != nil {
	// 				t.Log("There is an error : ", err)
	// 			}
	// 			// else {
	// 			// 	t.Log("Packet sent with bytes : ", n)
	// 			// }
	// 		}
	// 	}(conn)
	// }
	// waitgroup.Wait()
	t.Log("Timestamp after the testrun ends : ", time.Now())
	PrintMemUsage()
	// time.Sleep(10000 * time.Millisecond)
	// t.Log("After the TLS connection is established")
	// time.Sleep(300 * time.Millisecond)
	// assert.Equal(t, toalPackets, localServer.runningIndex, "All the messages are not processed.")
}

func TestServer_SphinxPacketSize(t *testing.T) {
	// if testing.Short() {
	//     t.Skip("skipping test in short mode.")
	// }

	sphinxPacket := createLargeTestPacket(t, fmt.Sprintf("%d : hello world", 343))
	bSphinxPacket, err := proto.Marshal(sphinxPacket)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("The size of one packet in bytes: ", len(bSphinxPacket))
}

// TestServer_CheckMultipleFunnels
// run integrationtest_preparation first!
func TestServer_CheckMultipleFunnels(t *testing.T) {
	db, err := pki.OpenDatabase("/home/olaf/GolandProjects/protocolX/"+PKI_DIR, "sqlite3")
	if err != nil {
		panic(err)
	}
	// row := db.QueryRow("SELECT Config FROM Pki WHERE Id = ? AND Typ = ?", "Provider"+strconv.Itoa(1), "provider")
	row := db.QueryRow("SELECT Id FROM Pki WHERE Typ = ?", "Provider")

	var results []byte
	err = row.Scan(&results)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(results)

	// Create Message and send it to funnel using compute node
}

func TestServer_EndToEnd(t *testing.T) {
	threadsCount = 2
	var connections = make([]net.Conn, threadsCount)

	for i := 0; i < threadsCount; i++ {
		t.Log("After the server starts")
		port := 9900 + i // compute node acts as provider (takes client messages)
		connections[i] = createTlsConnection(port, t)
		if connections[i] == nil {
			t.Log("Conn is nil")
		}

		t.Log("After the TLS connection is established")
		time.Sleep(30 * time.Millisecond)
	}
	sphinxPacket := createStaticTestPacket(t, "hello world")
	bSphinxPacket, err := proto.Marshal(sphinxPacket)
	if err != nil {
		t.Fatal(err)
	}
	totalPackets := 3
	t.Log("Timestamp before the testrun starts : ", time.Now())

	var waitgroup sync.WaitGroup
	for j := 0; j < threadsCount; j++ {
		waitgroup.Add(1)
		conn := connections[j]
		go func(connection net.Conn) {
			defer waitgroup.Done()
			for i := 0; i < totalPackets; i++ {
				_, err := connection.Write(bSphinxPacket)
				if err != nil {
					t.Log("There is an error : ", err)
				}
			}
		}(conn)
	}
	waitgroup.Wait()
	t.Log("Timestamp after the testrun ends : ", time.Now())
}

// PrintMemUsage outputs the current, total and OS memory being used. As well as the number
// of garage collection cycles completed.
func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// func TestServer_HandleConnection(t *testing.T) {
// 	serverConn, _ := net.Pipe()
// 	// errs := make(chan error, 1)
// 	// serverConn.Write([]byte("test"))
// 	go func() {
// 		err := localServer.handleConnection(serverConn)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		serverConn.Close()
// 	}()
// 	// serverConn.Close()
// }

func createTestPacket(t *testing.T, payload string) *sphinx.SphinxPacket {
	db, err := pki.OpenDatabase("/home/olaf/GolandProjects/protocolX/"+PKI_DIR, "sqlite3")
	if err != nil {
		panic(err)
	}
	row := db.QueryRow("SELECT Config FROM Pki WHERE Id = ? AND Typ = ?", "8", "Provider") // for TLSreceive test ID has to be out of range to ensure stable compute functionality

	var results []byte
	err = row.Scan(&results)
	if err != nil {
		fmt.Println(err)
	}

	var mixConfig config.MixConfig
	err = proto.Unmarshal(results, &mixConfig)

	path := config.E2EPath{IngressProvider: mixConfig, Mixes: []config.MixConfig{mixConfig}, EgressProvider: mixConfig}
	sphinxPacket, err := sphinx.PackForwardMessage(curve, path, []float64{0.1, 0.2, 0.3}, payload)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &sphinxPacket
}

func createLargeTestPacket(t *testing.T, payload string) *sphinx.SphinxPacket {
	path := config.E2EPath{IngressProvider: localServer.config, Mixes: []config.MixConfig{remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config, remoteServer.config}, EgressProvider: localServer.config}
	sphinxPacket, err := sphinx.PackForwardMessage(curve, path, []float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}, payload)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &sphinxPacket
}

func createTestPacketForDynamicFunnels(t *testing.T, payload string) *sphinx.SphinxPacket {
	funnels := helpers.GetCurrentFunnelNodes(globalNodeCount)
	db, err := pki.OpenDatabase("/home/olaf/GolandProjects/protocolX/"+PKI_DIR, "sqlite3")
	if err != nil {
		panic(err)
	}

	// make list with all nodes and remove funnels to get remaining compute nodes
	nodeIds := helpers.CreateRangedSlice(1, globalNodeCount) // nodeIds has to start with the same Ids as the server ID while spawning
	for _, funndelId := range funnels {
		helpers.RemoveIndexFromSlice(nodeIds, funndelId-1) //-1 because slices are 0-indexed
	}
	// get a random compute node
	randId := mrand.Int31n(int32(len(nodeIds) - 1))
	row := db.QueryRow("SELECT Config FROM Pki WHERE Id = ? AND Typ = ?", randId, "provider")

	var results []byte
	err = row.Scan(&results)
	if err != nil {
		fmt.Println(err)
	}
	var mixConfigCompute config.MixConfig
	err = proto.Unmarshal(results, &mixConfigCompute)

	// create packet
	path := config.E2EPath{IngressProvider: localServer.config, Mixes: []config.MixConfig{mixConfigCompute}, EgressProvider: localServer.config}
	sphinxPacket, err := sphinx.PackForwardMessage(curve, path, []float64{0.1, 0.2, 0.3}, payload)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &sphinxPacket
}

func createStaticTestPacket(t *testing.T, payload string) *sphinx.SphinxPacket {
	db, err := pki.OpenDatabase("/home/olaf/GolandProjects/protocolX/"+PKI_DIR, "sqlite3")
	if err != nil {
		panic(err)
	}

	// create configs for compute node
	row := db.QueryRow("SELECT Config FROM Pki WHERE Id = ? AND Typ = ?", "1", "provider")

	var results []byte
	err = row.Scan(&results)
	if err != nil {
		fmt.Println(err)
	}
	var computeConfig config.MixConfig
	err = proto.Unmarshal(results, &computeConfig)

	// create ClientConfig for recipient
	clientConfig := config.ClientConfig{Id: "1", Host: "localhost", Port: "50000", Provider: &localServer.config}

	// create packet
	path := config.E2EPath{IngressProvider: localServer.config, Mixes: []config.MixConfig{computeConfig}, EgressProvider: localServer.config, Recipient: clientConfig}
	sphinxPacket, err := sphinx.PackForwardMessage(curve, path, []float64{0.1, 0.2, 0.3}, payload)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &sphinxPacket
}

// func TestServer_ReceivedPacket(t *testing.T) {
// 	sphinxPacket := createTestPacket(t, "hello world")
// 	bSphinxPacket, err := proto.Marshal(sphinxPacket)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	err = localServer.receivedPacket(bSphinxPacket)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// }

/*
func Server_BatchProcessPacket(t *testing.T) {
// func TestServer_BatchProcessPacket(t *testing.T) {
	// packets := make([]node.MixPacket, len(p.aPac))
	// threads := runtime.GOMAXPROCS(0) -2

	threads := runtime.GOMAXPROCS(0) -1
	fmt.Println("test: the total number of threads used : ", threads)
	// logLocal.Info("main: case client:  the total number of threads used : ", threads)

	// roundLengths := []time.Duration{500 * time.Millisecond}
	// roundLengths := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond, 1000 * time.Millisecond}

	// for _, roundDuration := range roundLengths {
	fmt.Println("---------------------------------------- \n ")
	fmt.Println("test:  ROUND DURATION : ", config.RoundDuration)
	fmt.Println("---------------------------------------- \n ")
	testSizes := []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 140, 160, 180, 200, 220, 240, 260, 280, 300, 320, 340, 360, 380, 400, 420, 440, 460, 480, 500, 520, 540, 560, 580, 600}
	// testSizes := []int{2}
	for _, testSize := range testSizes {
		// testSize := 2
		fmt.Println("test:  batch size : ", testSize)
		// providerServer.aPac = make([]node.MixPacket, 0)
		localServer.aPac = make([]node.MixPacket, testSize, testSize)

		var waitgroup sync.WaitGroup

		dummyQueue := make(chan []byte, testSize)
		for j:=0; j<testSize; j++ {
			waitgroup.Add(1)
			// position := j
			go func() {
				defer waitgroup.Done()
				payload := strconv.Itoa(97)
				sphinxPacket := createTestPacket(t, payload)
				bSphinxPacket, err := proto.Marshal(sphinxPacket)
				if err != nil {
					t.Fatal(err)
				}
				// dummyQueue = append(dummyQueue, bSphinxPacket)
				dummyQueue <- bSphinxPacket
				// logLocal.Info("dummyQueue is appended with message number ", i)
			} ()
		}
		waitgroup.Wait()

		var wg sync.WaitGroup
		lengthAtStart := len(dummyQueue)
		wg.Add(testSize)
		delayBeforeContinute(config.RoundDuration, config.SyncTime)
		roundAtStart := config.GetRound()
		// fmt.Println("test: total messages processed : ", len(localServer.aPac))

		for i := 0; i<testSize; i++ {
			index := i
			go func() {
				defer wg.Done()
				// dummyPacket <- dummyQueue
				// if dummyPacket == nil {
				// 	t.Fatalf("Something wrong brother, why is the channel empty!")
				// }
				err := localServer.receivedPacketWithIndex(<- dummyQueue, index)
				if err != nil {
					t.Fatal(err)
				}
			} ()
			// err = localServer.receivedPacket(bSphinxPacket)
			// if err != nil {
			// 	t.Fatal(err)
			// }
		}

		delayBeforeContinute(config.RoundDuration, config.SyncTime)
		lengthOfChannel := len(dummyQueue)
		packets := make([]node.MixPacket, len(localServer.aPac))
		localServer.mutex.Lock()
		copy(packets, localServer.aPac)
		localServer.mutex.Unlock()
		// lengthAtEnd := countMixPackets(packets)

		wg.Wait()
		roundAtEnd := config.GetRound()
		fmt.Println("test: total messages processed after one round : ", countMixPackets(packets))
		fmt.Println("test: remaining packets in the channel after one round : ", lengthOfChannel)

		fmt.Println("test: total messages initially sent : ", lengthAtStart)
		fmt.Println("test: total messages processed after all the threads are done : ", countMixPackets(localServer.aPac))
		fmt.Println("test: The run started at round : ", roundAtStart)
		fmt.Println("test: The run ended at round : ", roundAtEnd)
		// if (roundAtEnd > roundAtStart+1) {
		// 	assert.Equal(t, roundAtStart+1, roundAtEnd, "The computation took more than one round.")
		// 	break
		// }
		// assert.Equal(t, roundAtStart+1, roundAtEnd, "The computation took more than one round.")
		// assert.Equal(t, testSize, len(localServer.aPac), "All the messages are not processed.")
	}
	// }
}
*/

func countMixPackets(packets []node.MixPacket) int {
	count := 0
	for i := 0; i < len(packets); i++ {
		if packets[i].Flag == "" {
			// fmt.Println("is zero value")
			continue
		}
		count++
	}
	return count
}

// func createNewTestPacket() *sphinx.SphinxPacket {
// 	path := config.E2EPath{IngressProvider: localServer.config, Mixes: []config.MixConfig{}, EgressProvider: anotherServer.config}
// 	sphinxPacket, err := sphinx.PackForwardMessage(elliptic.P224(), path, []float64{0.1, 0.2}, "Hello world")
// 	if err != nil {
// 		t.Fatal(err)
// 		return nil
// 	}
// 	return &sphinxPacket
// }

// func BenchmarkBigLen(b *testing.B) {
//     big := NewBig()
//     b.ResetTimer()
//     for i := 0; i < b.N; i++ {
//         big.Len()
//     }
// }
