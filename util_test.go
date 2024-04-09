package go_atomos

import "testing"

func BenchmarkBaseAtomos_GetGoID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getGoID()
	}
}
