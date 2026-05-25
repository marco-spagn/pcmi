package worker

import (
	"fmt"
	"testing"
)

var sinkString string
var sinkSlice []int64
var sinkBool bool

// BenchmarkDistillPathPrefix measures DistillPathPrefix with realistic path inputs.
func BenchmarkDistillPathPrefix(b *testing.B) {
	paths := make([]string, 1000)
	for i := range paths {
		paths[i] = fmt.Sprintf("root.user_%d.session_%d.event_%d", i%50, i%20, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString = DistillPathPrefix(paths[i%len(paths)])
	}
}

// BenchmarkNormalizeSourceIDs measures normalizeSourceIDs for slice sizes 10, 100, 1000.
func BenchmarkNormalizeSourceIDs(b *testing.B) {
	sizes := []int{10, 100, 1000}
	for _, n := range sizes {
		n := n
		ids := make([]int64, n)
		for i := range ids {
			ids[i] = int64(n - i)
		}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkSlice = normalizeSourceIDs(ids)
			}
		})
	}
}

// BenchmarkSourceIDsEqual compares sorted vs unsorted slices of 100 IDs.
func BenchmarkSourceIDsEqual(b *testing.B) {
	n := 100
	a := make([]int64, n)
	bSlice := make([]int64, n)
	for i := range a {
		a[i] = int64(i + 1)
		bSlice[i] = int64(n - i)
	}
	b.Run("sorted_vs_unsorted", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sinkBool = sourceIDsEqual(a, bSlice)
		}
	})
}
