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
	"crypto/aes"
	"crypto/cipher"
	// "crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	// "crypto/ed25519"
	Curve "golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"

	"math/big"
)

var P = big.NewInt(0).Sub(big.NewInt(0).Exp(big.NewInt(2), big.NewInt(255), nil), big.NewInt(19))

func AES_CTR(key, plaintext []byte) ([]byte, error) {

	ciphertext := make([]byte, len(plaintext))

	iv := []byte("0000000000000000")
	//if _, err := io.ReadFull(crand.Reader, iv); err != nil {
	//	panic(err)
	//}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext, plaintext)

	return ciphertext, nil
}

func hash(arg []byte) []byte {

	h := sha256.New()
	h.Write(arg)

	return h.Sum(nil)
}

func Hmac(key, message []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

func GenerateKeyPair() ([]byte, []byte, error) {
	priv, pub, err := box.GenerateKey(rand.Reader)

	if err != nil {
		return nil, nil, err
	}

	return pub[:], priv[:], nil
}

func KDF(key []byte) []byte {
	return hash(key)[:K]
}

func bytesToBigNum(value []byte) *big.Int {
	nBig := new(big.Int)
	nBig.SetBytes(value)

	return new(big.Int).Mod(nBig, P)
}

func randomBigInt() (big.Int, error) {
	nBig, err := rand.Int(rand.Reader, P)
	if err != nil {
		return big.Int{}, err
	}
	return *nBig, nil
}

func expo(base []byte, exp []big.Int) []byte {
	x := exp[0]
	for _, val := range exp[1:] {
		x = *big.NewInt(0).Mul(&x, &val)
	}

	// baseX, baseY := elliptic.Unmarshal(elliptic.P224(), base)
	// resultX, resultY := curve.Params().ScalarMult(baseX, baseY, x.Bytes())
	// return elliptic.Marshal(curve, resultX, resultY)

	s, err := Curve.X25519(x.Bytes(), base)

	if err != nil {
		logLocal.WithError(err).Error("Error in ProcessSphinxPacket - Group operation failed, probably invalid base.")
		return nil
	}
	return s
}

func expoGroupBase(exp []big.Int) []byte {
	x := exp[0]

	for _, val := range exp[1:] {
		x = *big.NewInt(0).Mul(&x, &val)
	}

	s, err := Curve.X25519(x.Bytes(), Curve.Basepoint)
	// resultX, resultY := curve.Params().ScalarBaseMult(x.Bytes())
	// return elliptic.Marshal(curve, resultX, resultY)
	if err != nil {
		logLocal.WithError(err).Error("Error in ProcessSphinxPacket - Group operation failed, probably invalid base.")
		return nil
	}
	
	return s
}

func computeMac(key, data []byte) []byte {
	return Hmac(key, data)
}
