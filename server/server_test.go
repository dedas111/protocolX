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
	"crypto/rand"
	"github.com/dedas111/protocolX/config"
	helpers "github.com/dedas111/protocolX/helpers"
	"github.com/dedas111/protocolX/node"
	"github.com/dedas111/protocolX/pki"
	sphinx "github.com/dedas111/protocolX/sphinx2"
	// "github.com/dedas111/protocolX/sphinx"
	"github.com/stretchr/testify/assert"
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

var packetCountTest int
var tsStart time.Time
var tsDone time.Time
var probingMode int

// var listOfComputeIPs = [...]string{"3.92.229.103"}
// var listOfComputeIPs = [...]string{"44.211.213.238","54.163.13.8"}
var listOfComputeIPs = [...]string{"44.211.213.238"}

const (
	testDatabase       = "testDatabase.db"
	remoteIP           = "54.92.157.34" // remote IP of compute for testing - not needed for standalone test because it uses multiple compute nodes
	localIP            = "3.92.229.103" // IP of the client receiving the packets for the tests
	threadsCountClient = 16                // listener threads on client
	threadsCountServer = 8                // listener threads on compute/server
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
	cert, err := tls.LoadX509KeyPair("/home/ec2-user/GolandProjects/protocolX/certs/client.pem", "/home/ec2-user/GolandProjects/protocolX/certs/client.key")
	if err != nil {
		t.Log("server: loadkeys")
		return nil
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	//conn, err := tls.Dial("tcp", "10.45.30.179:"+strconv.Itoa(port), &config)
	ip := remoteIP
	conn, err := tls.Dial("tcp", ip+":"+strconv.Itoa(port), &config)
	if conn == nil {
		t.Log("Conn is nil")
		t.Log(err)
		// retunr nil
	}
	// defer conn.Close()
	fmt.Println("client: connected to: ", conn.RemoteAddr())
	/*TestServer_TlsMemoryLoad
	state := conn.ConnectionState()
	for _, v := range state.PeerCertificates {
		fmt.Println(x509.MarshalPKIXPublicKey(v.PublicKey))
		fmt.Println(v.Subject)
	}
	t.Log("client: handshake: ", state.HandshakeComplete)
	t.Log("client: mutual: ", state.NegotiatedProtocolIsMutual)
	*/
	// message := "Hello\n"
	// n, err := io.WriteString(conn, message)
	// if err != nil {
	//     logLocal.Info("client: write: %s", err)
	// }
	// logLocal.Info("client: wrote %q (%d bytes)", message, n)
	return conn
}

func createTlsConnectionToIndividual(ip string, port int, t *testing.T) net.Conn {
	t.Log("Before client loadkeys")
	cert, err := tls.LoadX509KeyPair("/home/ec2-user/GolandProjects/protocolX/certs/client.pem", "/home/ec2-user/GolandProjects/protocolX/certs/client.key")
	if err != nil {
		t.Log("server: loadkeys")
		return nil
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	//conn, err := tls.Dial("tcp", "10.45.30.179:"+strconv.Itoa(port), &config)
	conn, err := tls.Dial("tcp", ip+":"+strconv.Itoa(port), &config)
	if conn == nil {
		t.Log("Conn is nil")
		// retunr nil
	}
	// defer conn.Close()
	t.Log("client: connected to: ", conn.RemoteAddr())

	// state := conn.ConnectionState()
	// for _, v := range state.PeerCertificates {
	// 	fmt.Println(x509.MarshalPKIXPublicKey(v.PublicKey))
	// 	fmt.Println(v.Subject)
	// }
	//fmt.Println("client: handshake: ", state.HandshakeComplete)
	//fmt.Println("client: mutual: ", state.NegotiatedProtocolIsMutual)

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

	threadsCount = 8
	var connections = make([]net.Conn, threadsCount)

	for i := 0; i < threadsCount; i++ {
		t.Log("After the server starts")
		// time.Sleep(20 * time.Millisecond)
		port := 9900 + i
		connections[i] = createTlsConnectionToIndividual("172.31.73.169", port, t)
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
	// assert.Equal(t, totalPackets, localServer.array, "All the messages are not processed.")
}

func TestServer_TlsMemoryLoad(t *testing.T) {
	// if testing.Short() {
	//     t.Skip("skipping test in short mode.")
	// }

	// t.Log("Before the server starts")
	// time.Sleep(300 * time.Millisecond)
	// localServer.startTlsServer()

	threadsCount = 4
	var connections = make([]net.Conn, threadsCount)

	for i := 0; i < threadsCount; i++ {
		t.Log("After the server starts")
		// time.Sleep(20 * time.Millisecond)
		port := 9900 + i
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
	// assert.Equal(t, toalPackets, localServer.array, "All the messages are not processed.")
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
/*
func TestServer_CheckMultipleFunnels(t *testing.T) {
	db, err := pki.OpenDatabase("/home/debajyoti/Documents/protocolX/"+PKI_DIR, "sqlite3")
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

*/


func TestServer_FunnelCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// t.Log("Before the server starts")
	// time.Sleep(300 * time.Millisecond)
	// localServer.startTlsServer()

	totalPackets := 60000 // sent per server Thread
	initialListenPort := 9900
	packetCountTest = totalPackets * threadsCountServer
	go createTestTLSListener(initialListenPort, t)
	time.Sleep(1 * time.Second)

	threadsCount = 8
	var connections = make([]net.Conn, threadsCount)

	for i := 0; i < threadsCount; i++ {
		t.Log("After the server starts")
		// time.Sleep(20 * time.Millisecond)
		port := 13000 + i
		connections[i] = createTlsConnectionToIndividual("172.31.73.169", port, t)
		// connections[i] = createTlsConnection(port, t)
		if connections[i] == nil {
			t.Log("Conn is nil")
			// retunr nil
		}

		t.Log("After the TLS connection is established")
		time.Sleep(30 * time.Millisecond)
	}

	// totalPackets := 5
	// sphinxPacket := createTestPacket(t, "hello world")
	// sphinxPacket := createLargeTestPacket(t, "hello world")
	// bSphinxPacket, err := proto.Marshal(sphinxPacket)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// t.Log("Timestamp before the testrun starts : ", time.Now())

	// sending begins
	// initialListenPort := 9900
	testPackages := make([][][]byte, threadsCount)
	for i, _ := range testPackages {
		testPackages[i] = make([][]byte, totalPackets)
	}
	for j := 0; j < threadsCount; j++ {
		for i := 0; i < totalPackets; i++ {
			payload := fmt.Sprintf("hello world %d %d ...................................................................................................................................................................................................................................................................................................................................................................................................................sfgkgafgsjksdvgjsgvadjkgvjgvkjvjV JV V VV hVV vvkjsVJVKS JV Vk VJSG VH vkhv .....................................................................................................................................................................................................................................................................................................................................................................................................................................vsn,nvsjkvskvlksklvnlkvslvsksmsvlkslvmsvkdlmvslkvsljjvsdljsl.", j, i)
			generalPacket := createTestPacketWithoutHops(t, payload, strconv.Itoa(initialListenPort))
			// fmt.Println("Packet:: ", generalPacket)
			bSphinxPacket, err := proto.Marshal(generalPacket)
			if err != nil {
				t.Fatal(err)
			}
			testPackages[j][i] = bSphinxPacket
		}
	}
	tsStart = time.Now()
	fmt.Println("Timestamp before sending starts : ", tsStart)

	var waitgroup sync.WaitGroup
	for j := 0; j < threadsCount; j++ {
		index := j
		waitgroup.Add(1)
		conn := connections[index]
		go func(connection net.Conn) {
			defer waitgroup.Done()
			for i := 0; i < totalPackets; i++ {
				_, err := connection.Write(testPackages[index][i])
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
	fmt.Println("Timestamp after the packets have all been sent: ", time.Now())
	// sleep timer to keep listener alive
	for tsDone.IsZero() {
		time.Sleep(time.Millisecond * 100)
	}
	fmt.Println("From sending to reception of all " + strconv.Itoa(packetCountTest) + " packets it took " + tsDone.Sub(tsStart).String() + ".")
}

// this test sends sphinx encrypted packets to servers and expects them to answer using a listener
func TestServer_EndToEndStandalone(t *testing.T) {
	totalPackets := 2500 // sent per Client Thread
	packetCountTest = totalPackets * threadsCountServer * len(listOfComputeIPs)
	go createTestTLSListener(50000, t)

	// create connection slice and create slices for actual connections
	var connections = make([][]net.Conn, len(listOfComputeIPs))
	for i, _ := range connections {
		connections[i] = make([]net.Conn, threadsCountServer)
	}
	for j, ip := range listOfComputeIPs {
		for i := 0; i < threadsCountServer; i++ {
			//t.Log("After the server starts")
			//fmt.Println("After the server starts")
			port := 9900 + i // compute node acts as provider (takes client messages)
			connections[j][i] = createTlsConnectionToIndividual(ip, port, t)
			if connections[j][i] == nil {
				t.Log("Conn is nil")
			}
			t.Log("After the TLS connection is established")
			time.Sleep(30 * time.Millisecond)
		}
	}
	// sending begins
	initialListenPort := 50000
	testPackages := make([][][]byte, len(listOfComputeIPs))
	for i, _ := range testPackages {
		testPackages[i] = make([][]byte, threadsCountServer)
	}
	for j, ip := range listOfComputeIPs {
		for i := 0; i < threadsCountServer; i++ {
			sphinxPacket := createStaticTestPacketWithPortForIndividual(t, "hello world", ip, strconv.Itoa(initialListenPort+i))
			bSphinxPacket, err := proto.Marshal(sphinxPacket)
			if err != nil {
				t.Fatal(err)
			}
			testPackages[j][i] = bSphinxPacket
		}
	}

	fmt.Println("Sending probe packets to " + strconv.Itoa(len(listOfComputeIPs)) + " compute nodes.")

	for k, ip := range listOfComputeIPs {
		for j := 0; j < threadsCountServer; j++ {
			index := j
			sphinxPacket := createStaticTestPacketWithPortForIndividual(t, "hello world", ip, strconv.Itoa(initialListenPort+j))
			bSphinxPacket, err := proto.Marshal(sphinxPacket)
			if err != nil {
				t.Fatal(err)
			}
			// waitgroup.Add(1)
			conn := connections[k][index]
			_, err = conn.Write(bSphinxPacket)
			if err != nil {
				t.Log("There is an error : ", err)
			}
		}
	}
	// waitgroup.Wait()

	for probingMode == 1 {
		time.Sleep(time.Millisecond * 100)
	}

	time.Sleep(time.Millisecond * 1000)
	
	fmt.Println("Sending " + strconv.Itoa(packetCountTest) + " packets to " + strconv.Itoa(len(listOfComputeIPs)) + " compute nodes.")
	tsStart = time.Now()
	fmt.Println("Timestamp before sending starts : ", tsStart)

	var waitgroup sync.WaitGroup

	for k, _ := range listOfComputeIPs {
		for j := 0; j < threadsCountServer; j++ {
			index := j
			waitgroup.Add(1)
			conn := connections[k][index]
			go func(connection net.Conn, computeIndex int, testPacketIndex int) {
				defer waitgroup.Done()
				for i := 0; i < totalPackets; i++ {
					//for countPackets < totalPackets {
					_, err := connection.Write(testPackages[computeIndex][testPacketIndex])
					if err != nil {
						t.Log("There is an error : ", err)
					}
					//countPackets++
					//t.Log(countPackets)
				}
			}(conn, k, index)
		}
	}
	waitgroup.Wait()
	fmt.Println("Timestamp after the packets have all been sent: ", time.Now())
	// sleep timer to keep listener alive
	for tsDone.IsZero() {
		time.Sleep(time.Millisecond * 100)
	}
	fmt.Println("From sending to reception of all " + strconv.Itoa(packetCountTest) + " packets it took " + tsDone.Sub(tsStart).String() + ".")
}

// run only if the server accepts unencrypted packets
/*
func TestServer_Unencrypted(t *testing.T) {
	go createTestTLSListener(50000, t)

	threadsCount = 1
	var connections = make([]net.Conn, threadsCount)

	for i := 0; i < threadsCount; i++ {
		//t.Log("After the server starts")
		fmt.Println("After the server starts")
		port := 9900 + i // compute node acts as provider (takes client messages)
		connections[i] = createTlsConnection(port, t)
		if connections[i] == nil {
			t.Log("Conn is nil")
		}
		t.Log("After the TLS connection is established")
		time.Sleep(30 * time.Millisecond)
	}

	totalPackets := 100 // sent per Client Thread
	t.Log("Timestamp before sending starts : ", time.Now())

	var waitgroup sync.WaitGroup
	for j := 0; j < threadsCount; j++ {
		waitgroup.Add(1)
		conn := connections[j]
		go func(connection net.Conn, index int) {
			defer waitgroup.Done()
			for i := 0; i < totalPackets; i++ {
				//for countPackets < totalPackets {
				payload := []byte(strconv.Itoa(i))
				t.Log("Sending packet with id ", i, " and bytes: ", payload)
				computePacket := config.ComputePacket{NextHop: "10.45.30.179:50000", Data: payload}
				bComputePacket, err := proto.Marshal(&computePacket)
				_, err = connection.Write(bComputePacket)
				if err != nil {
					t.Log("There is an error : ", err)
				}
			}
		}(conn, j)
	}
	waitgroup.Wait()
	t.Log("Timestamp after the packets have all been sent: ", time.Now())
	// sleep timer to keep listener alive
	time.Sleep(35000000000)
}

//		computePacket := config.ComputePacket{NextHop: "", Data: []byte(strconv.Itoa(ctr))}
//		bComputePacket, err := proto.Marshal(&computePacket)

*/

func TestServer_AddPacketsAndRearrange(t *testing.T) {
	packetCount := 1000
	threadCount := 5

	runningIndex := make([]int, threadCount)
	indexForRelay := make([]int, threadCount)
	receivedPackets := make([][][]byte, threadCount)
	for j := 0; j < threadCount; j++ {
		receivedPackets[j] = make([][]byte, packetCount*2)
	}
	outboundPackets := make([][]byte, 0)

	testServer := Server{receivedPackets: receivedPackets, runningIndex: runningIndex, indexSinceLastRelay: indexForRelay}

	ctr := 0
	var waitgroup sync.WaitGroup
	for j := 0; j < threadCount; j++ { // loop for threads
		waitgroup.Add(1)
		go func(threadId int, ctr int) {
			//time.Sleep(time.Duration(50 * ctr))
			defer waitgroup.Done()
			for i := 0; i < packetCount; i++ {
				payload := []byte(strconv.Itoa(ctr))
				computePacket := config.ComputePacket{NextHop: "54.91.206.62:50000", Data: payload}
				bComputePacket, err := proto.Marshal(&computePacket)
				if err != nil {
					panic(err)
				}
				testServer.receivedPacketWithIndex(bComputePacket, threadId)
				ctr++
			}
		}(j, ctr)
	}

	// rearranging here before waiting with go routine
	waitgroup.Add(1)
	go func(testServer Server) {
		defer waitgroup.Done()
		time.Sleep(200)
		packets := testServer.rearrangeReceivedPackets()
		outboundPackets = append(outboundPackets, packets...)
	}(testServer)

	waitgroup.Wait()
	rearrangedPackets := testServer.rearrangeReceivedPackets()
	outboundPackets = append(outboundPackets, rearrangedPackets...)
	assert.Equal(t, packetCount, testServer.indexSinceLastRelay[0])
	assert.Equal(t, packetCount*threadCount, len(outboundPackets))
	// ---------------------------- Second wave of packets ---------------------------- \\
	/*


		for j := 0; j < threadCount; j++ { // loop for threads
			waitgroup.Add(1)
			go func(threadId int, ctr int) {
				time.Sleep(time.Duration(500 * ctr))
				defer waitgroup.Done()
				for i := 0; i < packetCount; i++ {
					payload := []byte(strconv.Itoa(ctr))
					computePacket := config.ComputePacket{NextHop: "10.45.30.179:50000", Data: payload}
					bComputePacket, err := proto.Marshal(&computePacket)
					if err != nil {
						panic(err)
					}
					testServer.receivedPacketWithIndex(bComputePacket, threadId)
					ctr++
				}
			}(j, ctr)
		}

		// rearranging here before waiting with go routine
		waitgroup.Add(1)
		go func(testServer Server) {
			defer waitgroup.Done()
			packets := testServer.rearrangeReceivedPackets()
			outboundPackets = append(outboundPackets, packets...)
		}(testServer)

		waitgroup.Wait()
		rearrangedPackets = testServer.rearrangeReceivedPackets()
		outboundPackets = append(outboundPackets, rearrangedPackets...)
	*/

	//assert.Equal(t, 2*packetCount, testServer.indexSinceLastRelay[0])
	//assert.Equal(t, 2*packetCount*threadCount, len(outboundPackets))
}

func createTestTLSListener(initialListenPort int, t *testing.T) error {
	probingMode = 1
	
	// receivedPackets := 0
	receivedPackets := make([]int, threadsCountClient)
	for j := 0; j < threadsCountClient; j++ {
		receivedPackets[j] = 0
	}

	cert, err := tls.LoadX509KeyPair("/home/ec2-user/GolandProjects/protocolX/certs/server.pem", "/home/ec2-user/GolandProjects/protocolX/certs/server.key")
	if err != nil {
		t.Error("test: loadkeys error: ", err)
		panic(err)
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}}
	config.Rand = rand.Reader

	ip, err := helpers.GetLocalIP()
	// initialListenPort := 50000

	for someIndex := 0; someIndex < threadsCountClient; someIndex++ {
		loopPort := strconv.Itoa(initialListenPort + someIndex)
		listener, err := tls.Listen("tcp", ip+":"+loopPort, &config)
		if err != nil {
			t.Error("test: listen error: ", err)
			panic(err)
		}
		fmt.Println("test: listening on port:", loopPort)

		go func(localIndex int) {
			conn, err := listener.Accept()
			if err != nil {
				t.Error("test: accept error: ", err)
				panic(err)
				// break
			}
			//t.Log("test: accepted from ", conn.RemoteAddr())
			tlscon, ok := conn.(*tls.Conn)
			if ok {
				state := tlscon.ConnectionState()
				for _, v := range state.PeerCertificates {
					logLocal.Info(x509.MarshalPKIXPublicKey(v.PublicKey))
				}

			}
			buf := make([]byte, 1024)

			for {
				n, err := conn.Read(buf)
				if err != nil {
					t.Error("test: conn: read error: ", err)
					break
				}

				var answer sphinx.SphinxPacket
				proto.Unmarshal(buf[:n], &answer)
				if err != nil {
					t.Error("test: unmarshalling error: ", err)
					break
				}

				//t.Log("Received: ", string(buf[:n]))
				//t.Log("Packet received: ", answer)
				receivedPackets[localIndex]++

				// receivedPackets := make([]int, threadsCountClient)
				totalReceivedPackets := 0
				for j := 0; j < threadsCountClient; j++ {
					totalReceivedPackets = totalReceivedPackets + receivedPackets[j]
				}

				if probingMode == 1 {
					if totalReceivedPackets == threadsCountServer * len(listOfComputeIPs) {
						fmt.Println("Received all "+strconv.Itoa(totalReceivedPackets)+" probe packets at: ", time.Now())
						totalReceivedPackets = 0
						probingMode = 0
					}
				} else {
					// if totalReceivedPackets == int(float32(packetCountTest)*0.5) {
					// 	fmt.Println("Received 50% of all "+strconv.Itoa(packetCountTest)+" packets at: ", time.Now())
					// }
					if totalReceivedPackets == int(float32(packetCountTest)*0.9) {
						fmt.Println("Received 90% of all "+strconv.Itoa(packetCountTest)+" packets at: ", time.Now())
					}
					if totalReceivedPackets == int(float32(packetCountTest)*0.999) {
						fmt.Println("Received 90% of all "+strconv.Itoa(packetCountTest)+" packets at: ", time.Now())
					}
					if totalReceivedPackets == packetCountTest {
						tsDone = time.Now()
						fmt.Println("Received all "+strconv.Itoa(packetCountTest)+" packets at: ", tsDone)
					}
					//fmt.Println("Received packets: ", receivedPackets)
				}
			}
		}(someIndex)
	}
	return err
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
	db, err := pki.OpenDatabase("//home/ec2-user/GolandProjects/protocolX/"+PKI_DIR, "sqlite3")
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
	db, err := pki.OpenDatabase("/home/ec2-user/GolandProjects/protocolX/"+PKI_DIR, "sqlite3")
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
	row := db.QueryRow("SELECT Config FROM Pki WHERE Id = ? AND Typ = ?", randId, "Provider")

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

func createStaticTestPacketWithPort(t *testing.T, payload string, port string) *sphinx.SphinxPacket {
	var computeConfig config.MixConfig
	pubP, _ := os.ReadFile("/home/ec2-user/GolandProjects/protocolX/pki/pubP")
	computeConfig.Id = "1"
	computeConfig.Port = "9900"
	computeConfig.Host = remoteIP
	computeConfig.PubKey = pubP

	clientConfig := config.ClientConfig{Id: "1", Host: localIP, Port: port, Provider: &localServer.config}

	// create packet
	path := config.E2EPath{IngressProvider: computeConfig, Mixes: []config.MixConfig{computeConfig, computeConfig}, EgressProvider: computeConfig, Recipient: clientConfig}
	sphinxPacket, err := sphinx.PackForwardMessage(curve, path, []float64{0.1, 0.2, 0.3, 0.1, 0.2, 0.1}, payload)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &sphinxPacket
}

func createStaticTestPacketWithPortForIndividual(t *testing.T, payload string, ip string, port string) *sphinx.SphinxPacket {
	var computeConfig config.MixConfig
	pubP, _ := os.ReadFile("/home/ec2-user/GolandProjects/protocolX/pki/pubP")
	computeConfig.Id = "1"
	computeConfig.Port = "9900"
	computeConfig.Host = ip
	computeConfig.PubKey = pubP

	clientConfig := config.ClientConfig{Id: "1", Host: localIP, Port: port, Provider: &localServer.config}

	// create packet
	path := config.E2EPath{IngressProvider: computeConfig, Mixes: []config.MixConfig{computeConfig, computeConfig, computeConfig, computeConfig, computeConfig, computeConfig, computeConfig}, EgressProvider: computeConfig, Recipient: clientConfig}
	sphinxPacket, err := sphinx.PackForwardMessage(curve, path, []float64{0.1, 0.2, 0.3, 0.1, 0.2, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}, payload)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &sphinxPacket
}

func createTestPacketWithoutHops(t *testing.T, payload string, port string) *config.GeneralPacket {
	// var computeConfig config.MixConfig
	// pubP, _ := os.ReadFile("/home/debajyoti/Documents/protocolX/pki/pubP")
	// computeConfig.Id = "1"
	// computeConfig.Port = "9900"
	// computeConfig.Host = ip
	// computeConfig.PubKey = pubP

	// clientConfig := config.ClientConfig{Id: "1", Host: localIP, Port: port, Provider: &localServer.config}

	// create packet
	// bytes := make([]byte, 1000)
	// copy(bytes, payload)
	// pld := fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s", payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload, payload)
	bytes := []byte(payload)
	sphinxPacket := sphinx.SphinxPacket{Pld: bytes}
	bSphinxPacket, _ := proto.Marshal(&sphinxPacket)
	nexthop := fmt.Sprintf("%s:%s", localIP, port)
	computePacket := config.ComputePacket{NextHop: nexthop, Data: bSphinxPacket}
	bComputePacket, _ := proto.Marshal(&computePacket)
	generalPacket := config.GeneralPacket{Flag: commFlag, Data: bComputePacket}

	// path := config.E2EPath{Mixes: []config.MixConfig{}, Recipient: clientConfig}
	// sphinxPacket, err := sphinx.PackForwardMessage(curve, path, []float64{0.1}, payload)
	// if err != nil {
	// 	t.Fatal(err)
	// 	return nil
	// }
	return &generalPacket
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
				payload := strconv.Itoa(j)
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
