package atomos

import "testing"

func BenchmarkBaseAtomos_GetGoID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getGoID()
	}
}

//goos: darwin
//goarch: arm64
//pkg: github.com/hwangtou/go-atomos
//cpu: Apple M2 Max
//BenchmarkBaseAtomos_GetGoID
//BenchmarkBaseAtomos_GetGoID-12    	  645013	      1717 ns/op
//PASS
