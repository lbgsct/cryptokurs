package algorithm

import (
	"crypto/rand"
	"crypto/sha256"
	"math/big"
)

// генерирует большое простое число
func GeneratePrime(bits int) (*big.Int, error) {
	return rand.Prime(rand.Reader, bits)
}

// публичный ключ g^privateKey mod p
func GeneratePublicKey(g, privateKey, prime *big.Int) *big.Int {
	return new(big.Int).Exp(g, privateKey, prime)
}

// общий ключ (otherPublicKey ^ privateKey) mod p
func GenerateSharedKey(privateKey, otherPublicKey, prime *big.Int) *big.Int {
	return new(big.Int).Exp(otherPublicKey, privateKey, prime)
}

// хеширует общий ключ с помощью SHA-256
func HashSharedKey(sharedKey *big.Int) []byte {
	hash := sha256.New()
	hash.Write(sharedKey.Bytes())
	return hash.Sum(nil)
}
