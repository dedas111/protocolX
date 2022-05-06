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

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"

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
	priv, pub, err := box.GenerateKey(Reader)

	if err != nil {
		t.Error(err)
	}

	// randomPoint := elliptic.Marshal(curve, x, y)
	nBig := *big.NewInt(200000)
	exp := []big.Int{nBig}

	fmt.Println("Timestamp before exponentiation: ", time.Now())

	result := expo(pub, exp)

	fmt.Println("Timestamp after exponentiation: ", time.Now())

	_, x, y, err := elliptic.GenerateKey(curve, rand.Reader)

	if err != nil {
		t.Error(err)
	}

	fmt.Println("Timestamp before marshalling: ", time.Now())
	
	randomPoint := elliptic.Marshal(curve, x, y)

	fmt.Println("Timestamp before second exponentiation: ", time.Now())
	expectedX, expectedY := curve.ScalarMult(x, y, nBig.Bytes())
	fmt.Println("Timestamp after second exponentiation: ", time.Now())
	
	// assert.Equal(t, elliptic.Marshal(curve, expectedX, expectedY), result)

}
