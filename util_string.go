package atomos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"math/rand"
	"sync"
	"time"
)

const utilStringAZaz09 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const utilStringAZaz09Len = len(utilStringAZaz09)

type utilStringRandomStringGenerator struct {
	sync.Mutex
	rand *rand.Rand
}

func newUtilStringRandomStringGenerator() *utilStringRandomStringGenerator {
	return &utilStringRandomStringGenerator{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (u *utilStringRandomStringGenerator) RandomString(length int) string {
	u.Lock()
	defer u.Unlock()
	str := make([]byte, length)
	for i := 0; i < length; i++ {
		str[i] = utilStringAZaz09[u.rand.Intn(utilStringAZaz09Len-1)]
	}
	return string(str)
}

type utilStringHashGenerator struct {
	sync.Mutex
	hash hash.Hash
}

func newUtilStringHashSHA256Generator() *utilStringHashGenerator {
	return &utilStringHashGenerator{
		hash: sha256.New(),
	}
}

func (u *utilStringHashGenerator) Gen(content string) (string, *Error) {
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

func (u *utilStringHashGenerator) GenAny(content any) (string, *Error) {
	buf, er := json.Marshal(content)
	if er != nil {
		return "", NewErrorf(ErrUtilStringHashSHA256Failed, "UtilStringHashSHA256: Failed to marshal content. err=(%v)", er).AddStack(nil)
	}
	return u.Gen(string(buf))
}
