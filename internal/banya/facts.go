package banya

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
)

//go:embed facts.json
var DefaultFactsJSON []byte

type Fact struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

func LoadFacts(path string, embedded []byte) ([]Fact, error) {
	raw := embedded
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	var facts []Fact
	if err := json.Unmarshal(raw, &facts); err != nil {
		return nil, err
	}
	if len(facts) == 0 {
		return nil, fmt.Errorf("facts list is empty")
	}
	for _, f := range facts {
		if f.ID == 0 || f.Text == "" {
			return nil, fmt.Errorf("invalid fact: %+v", f)
		}
	}
	return facts, nil
}

func PickFact(facts []Fact, used map[int]struct{}, rng *rand.Rand) Fact {
	var unused []Fact
	for _, f := range facts {
		if _, ok := used[f.ID]; !ok {
			unused = append(unused, f)
		}
	}
	pool := unused
	if len(pool) == 0 {
		pool = facts
	}
	return pool[rng.Intn(len(pool))]
}
