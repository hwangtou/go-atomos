package go_atomos

import (
	"math/rand"
	"sync"
	"time"
)

const UtilStringAZaz09 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const UtilStringAZaz09Len = len(UtilStringAZaz09)

type UtilStringRandomStringGenerator struct {
	sync.Mutex
	*rand.Rand
}

func NewUtilStringRandomStringGenerator() *UtilStringRandomStringGenerator {
	return &UtilStringRandomStringGenerator{
		Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *UtilStringRandomStringGenerator) RandomString(length int) string {
	r.Lock()
	defer r.Unlock()
	str := make([]byte, length)
	for i := 0; i < length; i++ {
		str[i] = UtilStringAZaz09[r.Intn(UtilStringAZaz09Len-1)]
	}
	return string(str)
}
