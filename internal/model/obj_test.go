package model

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

// Keep the previous implementation as a reference for stability and compatibility.
func extractFolderReference(objs []Obj, mode string) {
	if mode == "" {
		return
	}
	front := mode == "front"
	sort.SliceStable(objs, func(i, j int) bool {
		if objs[i].IsDir() || objs[j].IsDir() {
			if !objs[i].IsDir() {
				return !front
			}
			if !objs[j].IsDir() {
				return front
			}
		}
		return false
	})
}

func TestExtractFolder(t *testing.T) {
	patterns := map[string][]bool{
		"empty":   nil,
		"file":    {false},
		"folder":  {true},
		"files":   {false, false, false},
		"folders": {true, true, true},
		"front":   {true, true, false, false},
		"back":    {false, false, true, true},
		"mixed":   {false, true, false, true, true, false},
	}
	for name, pattern := range patterns {
		for _, mode := range []string{"", "front", "back", "unknown"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				objs := make([]Obj, len(pattern))
				for i, folder := range pattern {
					objs[i] = &Object{Name: fmt.Sprint(i), IsFolder: folder}
				}
				want := append([]Obj{}, objs...)
				extractFolderReference(want, mode)
				ExtractFolder(objs, mode)
				if !reflect.DeepEqual(objs, want) {
					t.Fatalf("object order differs from stable reference: got %v, want %v", objs, want)
				}
			})
		}
	}
	t.Run("nil", func(t *testing.T) {
		var objs []Obj
		ExtractFolder(objs, "front")
		if objs != nil {
			t.Fatal("nil slice changed")
		}
	})
}

func TestExtractFolderRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 300; n++ {
		original := make([]Obj, n)
		for i := range original {
			original[i] = &Object{Name: fmt.Sprint(i), IsFolder: rng.Intn(2) == 0}
		}
		for _, mode := range []string{"front", "back", "unknown"} {
			got, want := append([]Obj{}, original...), append([]Obj{}, original...)
			ExtractFolder(got, mode)
			extractFolderReference(want, mode)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("n=%d mode=%q: order differs from stable reference", n, mode)
			}
		}
	}
}

func BenchmarkExtractFolder(b *testing.B) {
	for _, n := range []int{100, 10000} {
		for _, pattern := range []string{"mixed", "grouped", "files"} {
			original := make([]Obj, n)
			for i := range original {
				folder := pattern == "mixed" && i%3 == 0 || pattern == "grouped" && i < n/2
				original[i] = &Object{IsFolder: folder}
			}
			for _, impl := range []struct {
				name string
				fn   func([]Obj, string)
			}{{"reference", extractFolderReference}, {"optimized", ExtractFolder}} {
				b.Run(fmt.Sprintf("%s/%d/%s", pattern, n, impl.name), func(b *testing.B) {
					objs := make([]Obj, n)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						copy(objs, original)
						impl.fn(objs, "front")
					}
				})
			}
		}
	}
}
