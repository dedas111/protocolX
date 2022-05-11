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

package node

import (
	"github.com/dedas111/protocolX/config"
	sphinx "github.com/dedas111/protocolX/sphinx2"
	// "github.com/dedas111/protocolX/sphinx"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"

	"crypto/elliptic"
	
	"os"
	"reflect"
	"fmt"
	"sync"
    "time"
	"testing"
	// "strconv"
)

var nodes []config.MixConfig

func createProviderWorker() (*Mix, error) {
	pubP, privP, err := sphinx.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	providerWorker := NewMix(pubP, privP)
	return providerWorker, nil
}

func createTestPacket(curve elliptic.Curve, mixes []config.MixConfig, provider config.MixConfig, recipient config.ClientConfig) (*sphinx.SphinxPacket, error) {
	path := config.E2EPath{IngressProvider: provider, Mixes: mixes, EgressProvider: provider, Recipient: recipient}
	testPacket, err := sphinx.PackForwardMessage(curve, path, []float64{1.4, 2.5, 2.3, 3.2, 7.4}, "Test Message")
	if err != nil {
		return nil, err
	}
	return &testPacket, nil
}

func createTestMixes() ([]config.MixConfig, error) {
	pub1, _, err := sphinx.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	pub2, _, err := sphinx.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	pub3, _, err := sphinx.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	m1 := config.MixConfig{Id: "Mix1", Host: "localhost", Port: "3330", PubKey: pub1}
	m2 := config.MixConfig{Id: "Mix2", Host: "localhost", Port: "3331", PubKey: pub2}
	m3 := config.MixConfig{Id: "Mix2", Host: "localhost", Port: "3332", PubKey: pub3}
	nodes = []config.MixConfig{m1, m2, m3}

	return nodes, nil
}

func TestMain(m *testing.M) {

	os.Exit(m.Run())
}

func TestMixProcessPacket(t *testing.T) {
	ch := make(chan MixPacket, 1)
	errCh := make(chan error, 1)

	pubD, _, err := sphinx.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	providerWorker, err := createProviderWorker()
	if err != nil {
		t.Fatal(err)
	}
	provider := config.MixConfig{Id: "Provider", Host: "localhost", Port: "3333", PubKey: providerWorker.pubKey}
	dest := config.ClientConfig{Id: "Destination", Host: "localhost", Port: "3334", PubKey: pubD, Provider: &provider}
	mixes, err := createTestMixes()
	if err != nil {
		t.Fatal(err)
	}

	testPacket, err := createTestPacket(elliptic.P224(), mixes, provider, dest)
	if err != nil {
		t.Fatal(err)
	}

	testPacketBytes, err := proto.Marshal(testPacket)
	if err != nil {
		t.Fatal(err)
	}

	providerWorker.ProcessPacket(testPacketBytes, ch, errCh)
	dePacket := <-ch
	err = <-errCh
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, sphinx.Hop{Id: "Mix1", Address: "localhost:3330", PubKey: nodes[0].PubKey}, dePacket.Adr, "Next hop does not match")
	assert.Equal(t, reflect.TypeOf([]byte{}), reflect.TypeOf(dePacket.Data))
	assert.Equal(t, "\xF1", dePacket.Flag, reflect.TypeOf(dePacket.Data))
}

func TestMix_BatchProcessPacket(t *testing.T) {
	if testing.Short() {
        t.Skip("skipping test in short mode.")
    }
	// threads := runtime.GOMAXPROCS(0) -1
	// fmt.Println("test: the total number of threads used : ", threads)
	// logLocal.Info("main: case client:  the total number of threads used : ", threads)

	pubD, _, err := sphinx.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	providerWorker, err := createProviderWorker()
	if err != nil {
		t.Fatal(err)
	}
	provider := config.MixConfig{Id: "Provider", Host: "localhost", Port: "3333", PubKey: providerWorker.pubKey}
	dest := config.ClientConfig{Id: "Destination", Host: "localhost", Port: "3334", PubKey: pubD, Provider: &provider}
	mixes, err := createTestMixes()
	if err != nil {
		t.Fatal(err)
	}

	testSizes := []int{100, 200, 300, 400, 500, 600, 800, 1000, 1200, 1400, 1500, 1600, 1800, 2000, 2500, 3000}
	// testSizes := []int{2}
	for _, testSize := range testSizes {	
		// testSize := 2
		fmt.Println("test:  batch size : ", testSize)
		// localServer.aPac = make([]node.MixPacket, testSize, testSize)
	
		var waitgroup sync.WaitGroup
	
		dummyQueue := make([][]byte, testSize)
		for j:=0; j<testSize; j++ {
			waitgroup.Add(1)
			position := j
			go func(index int) {
				defer waitgroup.Done()
				// payload := strconv.Itoa(97)
				sphinxPacket, err := createTestPacket(elliptic.P224(), mixes, provider, dest)
				if err != nil {
					t.Fatal(err)
				}
				bSphinxPacket, err := proto.Marshal(sphinxPacket)
				if err != nil {
					t.Fatal(err)
				}
				// dummyQueue = append(dummyQueue, bSphinxPacket)
				dummyQueue[index] = bSphinxPacket
				// logLocal.Info("dummyQueue is appended with message number ", i)
			} (position)
		} 
		waitgroup.Wait()
	
		var wg sync.WaitGroup
		//lengthAtStart := len(dummyQueue)
		wg.Add(testSize)
		t.Log("Timestamp before the testrun starts : ", time.Now())
		// fmt.Println("test: total messages processed : ", len(localServer.aPac))
	
		for i := 0; i<testSize; i++ {
			index := i
			go func(position int) {
				defer wg.Done()
				// dummyPacket <- dummyQueue
				// if dummyPacket == nil {
				// 	t.Fatalf("Something wrong brother, why is the channel empty!")
				// }
				_, err := providerWorker.ProcessPacketInSameThread(dummyQueue[position])
				if err != nil {
					t.Fatal(err)
				}
			} (index)
			// err = localServer.receivedPacket(bSphinxPacket)
			// if err != nil {
			// 	t.Fatal(err)
			// }
		}
	
		wg.Wait()
		t.Log("Timestamp after the testrun ends : ", time.Now())
	}
}

func TestMix_Shuffle(t *testing.T) {
	providerWorker, err := createProviderWorker()
	if err != nil {
		t.Fatal(err)
	}

	testSizes := []int{200000, 400000, 600000, 800000, 1000000}
	// testSizes := []int{2}
	for _, testSize := range testSizes {
		numPackets = testSize
		fmt.Println("test:  batch size : ", numPackets)
		for i := 0; i < numPackets; i++ {
			randTable[i] = 5
			shuffled[i] = i
		}

		fmt.Println("Timestamp before the testrun stats : ", time.Now())
		providerWorker.preprocessShuffle()
		fmt.Println("Timestamp after the testrun ends : ", time.Now())
	}
}
