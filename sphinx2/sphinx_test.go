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

package sphinx2

import (
	"github.com/dedas111/protocolX/config"
	"time"

	"crypto/aes"
	"crypto/elliptic"
	"crypto/rand"

	Curve "golang.org/x/crypto/curve25519"
	// "golang.org/x/crypto/nacl/box"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"

	"fmt"
	"math/big"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	curve = elliptic.P224()

	os.Exit(m.Run())
}

func TestExpoSingleValue(t *testing.T) {
	pub, _, err := GenerateKeyPair() 

	if err != nil {
		t.Error(err)
	}

	// randomPoint := elliptic.Marshal(curve, x, y)
	// nBig := *big.NewInt(200000)
	nBig, err := rand.Int(rand.Reader, P)
	exp := []big.Int{*nBig}
	// exp := big.Int{nBig}

	fmt.Println("Timestamp before exponentiation: ", time.Now())

	pubB := pub[:]
	// privB := priv[:]
	expo(pubB, exp)

	fmt.Println("Timestamp after exponentiation: ", time.Now())
	fmt.Println("The scalar factor: ", nBig)
	fmt.Println("The scalar factor: ", *nBig)

	// _, x, y, err := elliptic.GenerateKey(curve, rand.Reader)

	// if err != nil {
	// 	t.Error(err)
	// }

	// fmt.Println("Timestamp before marshalling: ", time.Now())
	
	// randomPoint := elliptic.Marshal(curve, x, y)

	// fmt.Println("Timestamp before second exponentiation: ", time.Now())
	// expectedX, expectedY := curve.ScalarMult(x, y, nBig.Bytes())
	// fmt.Println("Timestamp after second exponentiation: ", time.Now())
	
	// assert.Equal(t, elliptic.Marshal(curve, expectedX, expectedY), result)

}

func TestHash(t *testing.T) {
	_, x, err := GenerateKeyPair()

	if err != nil {
		t.Error(err)
	}

	// randomPoint := elliptic.Marshal(curve, x, y)
	hVal := hash(x)

	assert.Equal(t, 32, len(hVal))

}

func TestBytesToBigNum(t *testing.T) {
	bytes := big.NewInt(100).Bytes()
	result := *bytesToBigNum(bytes)
	assert.Equal(t, *big.NewInt(100), result)
}

func TestGetAESKey(t *testing.T) {
	_, x, err := GenerateKeyPair()

	if err != nil {
		t.Error(err)
	}

	// randomPoint := elliptic.Marshal(curve, x, y)
	aesKey := KDF(x)
	assert.Equal(t, aes.BlockSize, len(aesKey))
}

func TestComputeBlindingFactor(t *testing.T) {
	// generator := elliptic.Marshal(curve, curve.Params().Gx, curve.Params().Gy)
	_, x, err := GenerateKeyPair()

	if err != nil {
		t.Error(err)
	}

	key := hash(x)
	b, err := computeBlindingFactor(key)
	
	if err != nil {
		t.Error(err)
	}

	fmt.Println("The blinding factor: ", b)

	// expected := new(big.Int)
	// expected.SetString("252286146058081748716688845275111486959", 10)

	// assert.Equal(t, expected, b)
}

func TestGetSingleSharedSecret(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	x, err := randomBigInt()
	blindFactors := []big.Int{x}

	if err != nil {
		t.Error(err)
	}

	alpha := expoGroupBase(blindFactors)
	s := expo(pub, blindFactors)

	sharedSecret, err := Curve.X25519(priv, alpha)

	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, s, sharedSecret)
}

func TestGetSharedSecrets(t *testing.T) {

	pub1, _, err := GenerateKeyPair()
	pub2, _, err := GenerateKeyPair()
	pub3, _, err := GenerateKeyPair()
	if err != nil {
		t.Error(err)
	}

	pubs := [][]byte{pub1, pub2, pub3}

	m1 := config.MixConfig{Id: "", Host: "", Port: "", PubKey: pub1}
	m2 := config.MixConfig{Id: "", Host: "", Port: "", PubKey: pub2}
	m3 := config.MixConfig{Id: "", Host: "", Port: "", PubKey: pub3}

	nodes := []config.MixConfig{m1, m2, m3}

	// x := big.NewInt(100)
	// x, err := GenerateKeyPair()
	x, err := randomBigIntBytes()


	if err != nil {
		t.Error(err)
	}

	fmt.Println("x: ", x)

	result, err := getSharedSecretsBytes(curve, nodes, x)
	if err != nil {
		t.Error(err)
	}

	fmt.Println("After GetSharedSecrets, x: ", x)

	var expected []HeaderInitials
	blindFactors := [][]byte{x}

	v := x
	// alpha0X, alpha0Y := curve.Params().ScalarMult(curve.Params().Gx, curve.Params().Gy, v.Bytes())
	// alpha0 := elliptic.Marshal(curve, alpha0X, alpha0Y)
	alpha0, err := Curve.X25519(v, Curve.Basepoint)
	s0 := expoBytes(pubs[0], blindFactors)
	aesS0 := KDF(s0)

	fmt.Println("s0: ", s0)

	b0, err := computeBlindingFactorBytes(aesS0)
	if err != nil {
		t.Error(err)
	}

	fmt.Println("b0: ", b0)

	expected = append(expected, HeaderInitials{Alpha: alpha0, Secret: s0, Blinder: b0, SecretHash: aesS0})
	blindFactors = append(blindFactors, b0)

	// v = big.NewInt(0).Mul(v, b0)
	// alpha1X, alpha1Y := curve.Params().ScalarMult(curve.Params().Gx, curve.Params().Gy, v.Bytes())
	alpha1, err := Curve.X25519(b0, alpha0)
	s1 := expoBytes(pubs[1], blindFactors)

	fmt.Println("s1: ", s1)

	aesS1 := KDF(s1)
	b1, err := computeBlindingFactorBytes(aesS1)
	if err != nil {
		t.Error(err)
	}

	fmt.Println("b1: ", b1)

	expected = append(expected, HeaderInitials{Alpha: alpha1, Secret: s1, Blinder: b1, SecretHash: aesS1})
	blindFactors = append(blindFactors, b1)

	// v = big.NewInt(0).Mul(v, b1)
	// alpha2X, alpha2Y := curve.Params().ScalarMult(curve.Params().Gx, curve.Params().Gy, v.Bytes())
	alpha2, err := Curve.X25519(b1, alpha1)
	s2 := expoBytes(pubs[2], blindFactors)

	fmt.Println("s2: ", s2)

	aesS2 := KDF(s2)
	b2, err := computeBlindingFactorBytes(aesS2)
	if err != nil {
		t.Error(err)
	}

	fmt.Println("b2: ", b2)

	expected = append(expected, HeaderInitials{Alpha: alpha2, Secret: s2, Blinder: b2, SecretHash: aesS2})
	blindFactors = append(blindFactors, b2)

	assert.Equal(t, expected, result)
}


func TestEncapsulateHeader(t *testing.T) {

	pub1, _, err := GenerateKeyPair()
	pub2, _, err := GenerateKeyPair()
	pub3, _, err := GenerateKeyPair()
	pubD, _, err := GenerateKeyPair()
	if err != nil {
		t.Error(err)
	}

	m1 := config.NewMixConfig("Node1", "localhost", "3331", pub1)
	m2 := config.NewMixConfig("Node2", "localhost", "3332", pub2)
	m3 := config.NewMixConfig("Node3", "localhost", "3333", pub3)

	nodes := []config.MixConfig{m1, m2, m3}

	c1 := Commands{Delay: 0.34, Flag: []byte("0")}
	c2 := Commands{Delay: 0.25, Flag: []byte("1")}
	c3 := Commands{Delay: 1.10, Flag: []byte("1")}
	commands := []Commands{c1, c2, c3}

	x, err := randomBigIntBytes()
	sharedSecrets, err := getSharedSecretsBytes(curve, nodes, x)
	if err != nil {
		t.Error(err)
	}

	actualHeader, err := encapsulateHeader(sharedSecrets, nodes, commands,
		config.ClientConfig{Id: "DestinationId", Host: "DestinationAddress", Port: "9998", PubKey: pubD})
	if err != nil {
		t.Error(err)
	}

	routing1 := RoutingInfo{NextHop: &Hop{"DestinationId", "DestinationAddress:9998", []byte{}}, RoutingCommands: &c3,
		NextHopMetaData: []byte{}, Mac: []byte{}}

	routing1Bytes, err := proto.Marshal(&routing1)
	if err != nil {
		t.Error(err)
	}

	enc_routing1, err := AES_CTR(KDF(sharedSecrets[2].SecretHash), routing1Bytes)
	if err != nil {
		t.Error(err)
	}

	mac1 := computeMac(KDF(sharedSecrets[2].SecretHash), enc_routing1)

	routing2 := RoutingInfo{NextHop: &Hop{"Node3", "localhost:3333", pub3}, RoutingCommands: &c2,
		NextHopMetaData: enc_routing1, Mac: mac1}

	routing2Bytes, err := proto.Marshal(&routing2)
	if err != nil {
		t.Error(err)
	}

	enc_routing2, err := AES_CTR(KDF(sharedSecrets[1].SecretHash), routing2Bytes)
	if err != nil {
		t.Error(err)
	}

	mac2 := computeMac(KDF(sharedSecrets[1].SecretHash), enc_routing2)

	expectedRouting := RoutingInfo{NextHop: &Hop{"Node2", "localhost:3332", pub2}, RoutingCommands: &c1,
		NextHopMetaData: enc_routing2, Mac: mac2}

	expectedRoutingBytes, err := proto.Marshal(&expectedRouting)
	if err != nil {
		t.Error(err)
	}

	enc_expectedRouting, err := AES_CTR(KDF(sharedSecrets[0].SecretHash), expectedRoutingBytes)
	if err != nil {
		t.Error(err)
	}

	mac3 := computeMac(KDF(sharedSecrets[0].SecretHash), enc_expectedRouting)

	expectedHeader := Header{sharedSecrets[0].Alpha, enc_expectedRouting, mac3}

	assert.Equal(t, expectedHeader, actualHeader)
}


func TestProcessSphinxHeader(t *testing.T) {

	pub1, priv1, err := GenerateKeyPair()
	pub2, _, err := GenerateKeyPair()
	pub3, _, err := GenerateKeyPair()
	if err != nil {
		t.Error(err)
	}

	c1 := Commands{Delay: 0.34}
	c2 := Commands{Delay: 0.25}
	c3 := Commands{Delay: 1.10}

	m1 := config.NewMixConfig("Node1", "localhost", "3331", pub1)
	m2 := config.NewMixConfig("Node2", "localhost", "3332", pub2)
	m3 := config.NewMixConfig("Node3", "localhost", "3333", pub3)

	nodes := []config.MixConfig{m1, m2, m3}

	x, err := randomBigIntBytes()
	sharedSecrets, err := getSharedSecretsBytes(curve, nodes, x)
	if err != nil {
		t.Error(err)
	}

	// Intermediate steps, which are needed to check whether the processing of the header was correct
	routing1 := RoutingInfo{NextHop: &Hop{"DestinationId", "DestinationAddress", []byte{}}, RoutingCommands: &c3,
		NextHopMetaData: []byte{}, Mac: []byte{}}

	routing1Bytes, err := proto.Marshal(&routing1)
	if err != nil {
		t.Error(err)
	}

	enc_routing1, err := AES_CTR(KDF(sharedSecrets[2].SecretHash), routing1Bytes)
	if err != nil {
		t.Error(err)
	}

	mac1 := computeMac(KDF(sharedSecrets[2].SecretHash), enc_routing1)

	routing2 := RoutingInfo{NextHop: &Hop{"Node3", "localhost:3333", pub3}, RoutingCommands: &c2,
		NextHopMetaData: enc_routing1, Mac: mac1}

	routing2Bytes, err := proto.Marshal(&routing2)
	if err != nil {
		t.Error(err)
	}

	enc_routing2, err := AES_CTR(KDF(sharedSecrets[1].SecretHash), routing2Bytes)
	if err != nil {
		t.Error(err)
	}

	mac2 := computeMac(KDF(sharedSecrets[1].SecretHash), enc_routing2)

	routing3 := RoutingInfo{NextHop: &Hop{"Node2", "localhost:3332", pub2}, RoutingCommands: &c1,
		NextHopMetaData: enc_routing2, Mac: mac2}

	routing3Bytes, err := proto.Marshal(&routing3)
	if err != nil {
		t.Error(err)
	}

	enc_expectedRouting, err := AES_CTR(KDF(sharedSecrets[0].SecretHash), routing3Bytes)
	if err != nil {
		t.Error(err)
	}

	mac3 := computeMac(KDF(sharedSecrets[0].SecretHash), enc_expectedRouting)

	header := Header{sharedSecrets[0].Alpha, enc_expectedRouting, mac3}

	tsStart := time.Now()
	nextHop, newCommands, newHeader, err := ProcessSphinxHeader(header, priv1)
	tsDone := time.Now()
	fmt.Println("Processing header took " + tsDone.Sub(tsStart).String() + ".")
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, nextHop, Hop{Id: "Node2", Address: "localhost:3332", PubKey: pub2})
	assert.Equal(t, newCommands, c1)
	assert.Equal(t, newHeader, Header{Alpha: sharedSecrets[1].Alpha, Beta: enc_routing2, Mac: mac2})

}

func TestProcessSphinxPayload(t *testing.T) {

	message := "Plaintext message"

	pub1, priv1, err := GenerateKeyPair()
	pub2, priv2, err := GenerateKeyPair()
	pub3, priv3, err := GenerateKeyPair()
	if err != nil {
		t.Error(err)
	}

	m1 := config.NewMixConfig("Node1", "localhost", "3331", pub1)
	m2 := config.NewMixConfig("Node2", "localhost", "3332", pub2)
	m3 := config.NewMixConfig("Node3", "localhost", "3333", pub3)

	nodes := []config.MixConfig{m1, m2, m3}

	x, err := randomBigIntBytes()
	asb, err := getSharedSecretsBytes(curve, nodes, x)
	if err != nil {
		t.Error(err)
	}

	encMsg, err := encapsulateContent(asb, message)
	if err != nil {
		t.Error(err)
	}

	var decMsg []byte

	decMsg = encMsg
	privs := [][]byte{priv1, priv2, priv3}
	tsStart := time.Now()
	for i, v := range privs {
		decMsg, err = ProcessSphinxPayload(asb[i].Alpha, decMsg, v)
		if err != nil {
			t.Error(err)
		}
	}
	tsDone := time.Now()
	fmt.Println("Processing payload took " + tsDone.Sub(tsStart).String() + ".")
	assert.Equal(t, []byte(message), decMsg)
}

func TestOnionEncryptThenDecrypt(t *testing.T) {
	pub1, priv1, err := GenerateKeyPair()
	pub2, priv2, err := GenerateKeyPair()
	pub3, priv3, err := GenerateKeyPair()
	pub4, priv4, err := GenerateKeyPair()
	if err != nil {
		t.Error(err)
	}

	message := "Plaintext message"

	c1 := Commands{Delay: 0.34}
	c2 := Commands{Delay: 0.25}
	c3 := Commands{Delay: 1.10}
	// c4 := Commands{Delay: 0.95}

	delays := []float64{0.34, 0.25, 1.10, 0.95}

	m1 := config.NewMixConfig("Node1", "localhost", "3331", pub1)
	m2 := config.NewMixConfig("Node2", "localhost", "3332", pub2)
	m3 := config.NewMixConfig("Node3", "localhost", "3333", pub3)
	dest := config.NewMixConfig("Destination", "DestinationHost", "3000", pub4)

	nodes := []config.MixConfig{m2, m3}

	path := config.E2EPath{IngressProvider: m1, Mixes: nodes, EgressProvider: dest}

	sphinxPacket, err := PackForwardMessage(curve, path, delays, message)
	sphinxPacketBytes, err := proto.Marshal(&sphinxPacket)

	hop1, command1, step1, err := ProcessSphinxPacket(sphinxPacketBytes, priv1)

	// var nextHop Hop
	// var newCommands Commands

	// err = proto.Unmarshal(hop1, &nextHop)
	// err = proto.Unmarshal(command1, &newCommands)
	// if err != nil {
	// 	t.Error(err)
	// }

	assert.Equal(t, Hop{Id: "Node2", Address: "localhost:3332", PubKey: pub2}, hop1)
	assert.Equal(t, c1.Delay, command1.Delay)

	hop2, command2, step2, err := ProcessSphinxPacket(step1, priv2)

	// err = proto.Unmarshal(hop2, &nextHop)
	// err = proto.Unmarshal(command2, &newCommands)
	// if err != nil {
	// 	t.Error(err)
	// }

	assert.Equal(t, Hop{Id: "Node3", Address: "localhost:3333", PubKey: pub3}, hop2)
	assert.Equal(t, c2.Delay, command2.Delay)

	hop3, command3, step3, err := ProcessSphinxPacket(step2, priv3)

	// err = proto.Unmarshal(hop3, &nextHop)
	// err = proto.Unmarshal(command3, &newCommands)
	// if err != nil {
	// 	t.Error(err)
	// }

	assert.Equal(t, Hop{Id: "Destination", Address: "DestinationHost:3000", PubKey: pub4}, hop3)
	assert.Equal(t, c3.Delay, command3.Delay)

	_, command4, step4, err := ProcessSphinxPacket(step3, priv4)

	fmt.Println("The packet at the recipient looks like:: Command.Delay : ", command4.Delay)

	// var decMsg []byte
	var finalPacket SphinxPacket
	err = proto.Unmarshal(step4, &finalPacket)

	fmt.Println("The content of the packet: ", string(finalPacket.Pld))

	assert.Equal(t, []byte(message), finalPacket.Pld)
}


func TestOnionEncryptThenDecryptLongMessage(t *testing.T) {
	pub1, priv1, err := GenerateKeyPair()
	pub2, priv2, err := GenerateKeyPair()
	pub3, priv3, err := GenerateKeyPair()
	pub4, priv4, err := GenerateKeyPair()
	if err != nil {
		t.Error(err)
	}

	message := "I have to write a long plaintext message, but I do not know what to write. So I start writing this story of a programmer who wanted to implement Sphinx packet structure in Golang. But Golang is not easy -- it has a lot of weird datatype. He wanted to give up many times, but he did not. Finally he succeed. He succeed to implement the Sphinx packet structure in Golang. Now he is happy."

	c1 := Commands{Delay: 0.34}
	c2 := Commands{Delay: 0.25}
	c3 := Commands{Delay: 1.10}
	// c4 := Commands{Delay: 0.95}

	delays := []float64{0.34, 0.25, 1.10, 0.95}

	m1 := config.NewMixConfig("Node1", "localhost", "3331", pub1)
	m2 := config.NewMixConfig("Node2", "localhost", "3332", pub2)
	m3 := config.NewMixConfig("Node3", "localhost", "3333", pub3)
	dest := config.NewMixConfig("Destination", "DestinationHost", "3000", pub4)

	nodes := []config.MixConfig{m2, m3}

	path := config.E2EPath{IngressProvider: m1, Mixes: nodes, EgressProvider: dest}

	sphinxPacket, err := PackForwardMessage(curve, path, delays, message)
	sphinxPacketBytes, err := proto.Marshal(&sphinxPacket)

	tsStart := time.Now()
	hop1, command1, step1, err := ProcessSphinxPacket(sphinxPacketBytes, priv1)
	tsDone := time.Now()
	fmt.Println("Processing step1 took " + tsDone.Sub(tsStart).String() + ".")

	// var nextHop Hop
	// var newCommands Commands

	// err = proto.Unmarshal(hop1, &nextHop)
	// err = proto.Unmarshal(command1, &newCommands)
	// if err != nil {
	// 	t.Error(err)
	// }

	assert.Equal(t, Hop{Id: "Node2", Address: "localhost:3332", PubKey: pub2}, hop1)
	assert.Equal(t, c1.Delay, command1.Delay)

	tsStart = time.Now()
	hop2, command2, step2, err := ProcessSphinxPacket(step1, priv2)
	tsDone = time.Now()
	fmt.Println("Processing step2 took " + tsDone.Sub(tsStart).String() + ".")

	// err = proto.Unmarshal(hop2, &nextHop)
	// err = proto.Unmarshal(command2, &newCommands)
	// if err != nil {
	// 	t.Error(err)
	// }

	assert.Equal(t, Hop{Id: "Node3", Address: "localhost:3333", PubKey: pub3}, hop2)
	assert.Equal(t, c2.Delay, command2.Delay)

	tsStart = time.Now()
	hop3, command3, step3, err := ProcessSphinxPacket(step2, priv3)
	tsDone = time.Now()
	fmt.Println("Processing step3 took " + tsDone.Sub(tsStart).String() + ".")

	// err = proto.Unmarshal(hop3, &nextHop)
	// err = proto.Unmarshal(command3, &newCommands)
	// if err != nil {
	// 	t.Error(err)
	// }

	assert.Equal(t, Hop{Id: "Destination", Address: "DestinationHost:3000", PubKey: pub4}, hop3)
	assert.Equal(t, c3.Delay, command3.Delay)

	tsStart = time.Now()
	_, command4, step4, err := ProcessSphinxPacket(step3, priv4)
	tsDone = time.Now()
	fmt.Println("Processing step4 took " + tsDone.Sub(tsStart).String() + ".")

	fmt.Println("The packet at the recipient looks like:: Command.Delay : ", command4.Delay)

	// var decMsg []byte
	var finalPacket SphinxPacket
	err = proto.Unmarshal(step4, &finalPacket)

	fmt.Println("The content of the packet: ", string(finalPacket.Pld))

	assert.Equal(t, []byte(message), finalPacket.Pld)
}






