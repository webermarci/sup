package sup

import "testing"

func BenchmarkRouter_Next_RoundRobin(b *testing.B) {
	r := NewRouter(RoundRobin, 1, 2, 3, 4, 5, 6, 7, 8)
	for b.Loop() {
		_ = r.Next()
	}
}

func BenchmarkRouter_Next_Random(b *testing.B) {
	r := NewRouter(Random, 1, 2, 3, 4, 5, 6, 7, 8)
	for b.Loop() {
		_ = r.Next()
	}
}

func BenchmarkRouter_Next_Parallel(b *testing.B) {
	r := NewRouter(RoundRobin, 1, 2, 3, 4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = r.Next()
		}
	})
}
