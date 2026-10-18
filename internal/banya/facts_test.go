package banya

import (
	"math/rand"
	"testing"
)

func TestPickFactSkipsUsed(t *testing.T) {
	facts := []Fact{{ID: 1, Text: "a"}, {ID: 2, Text: "b"}}
	used := map[int]struct{}{1: {}}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 10; i++ {
		f := PickFact(facts, used, rng)
		if f.ID != 2 {
			t.Fatalf("got id %d", f.ID)
		}
	}
}

func TestPickFactResetsWhenAllUsed(t *testing.T) {
	facts := []Fact{{ID: 1, Text: "a"}}
	used := map[int]struct{}{1: {}}
	f := PickFact(facts, used, rand.New(rand.NewSource(1)))
	if f.ID != 1 {
		t.Fatalf("got %d", f.ID)
	}
}
