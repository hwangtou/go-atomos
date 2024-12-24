package go_atomos

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"math/rand"
	"sync"
	"time"
)

const UtilStringAZaz09 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const UtilStringAZaz09Len = len(UtilStringAZaz09)

type UtilStringRandomStringGenerator struct {
	sync.Mutex
	rand *rand.Rand
}

func NewUtilStringRandomStringGenerator() *UtilStringRandomStringGenerator {
	return &UtilStringRandomStringGenerator{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (u *UtilStringRandomStringGenerator) RandomString(length int) string {
	u.Lock()
	defer u.Unlock()
	str := make([]byte, length)
	for i := 0; i < length; i++ {
		str[i] = UtilStringAZaz09[u.rand.Intn(UtilStringAZaz09Len-1)]
	}
	return string(str)
}

type UtilStringHashGenerator struct {
	sync.Mutex
	hash hash.Hash
}

func NewUtilStringHashSHA256Generator() *UtilStringHashGenerator {
	return &UtilStringHashGenerator{
		hash: sha256.New(),
	}
}

func (u *UtilStringHashGenerator) Gen(content string) (string, *Error) {
	u.Lock()
	defer u.Unlock()
	// Get the hash of the pre-generated file content.
	// Write the serialized buf to the hash.
	_, er := u.hash.Write([]byte(content))
	if er != nil {
		return "", NewErrorf(ErrUtilStringHashSHA256Failed, "UtilStringHashSHA256: Failed to write content to hash. err=(%v)", er).AddStack(nil)
	}
	// Calculate the hash and get the resulting byte slice.
	hashBytes := u.hash.Sum(nil)
	// Encode the hash to a hexadecimal string.
	hashString := hex.EncodeToString(hashBytes)
	u.hash.Reset()
	return hashString, nil
}
