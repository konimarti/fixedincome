package term

import (
	"encoding/json"
	"fmt"
)

var (
	registered = map[Structure][]string{
		&NelsonSiegelSvensson{}: []string{"b0", "b1", "b2", "b3", "t1", "t2", "spread"},
		&Flat{}:                 []string{"r", "spread"},
		&Spline{}:               []string{"maturities", "discountfactors", "spread"},
	}
)

type Initer interface {
	Init() error
}

func Parse(data []byte) (Structure, error) {
	// unmarshal data into map[string]interface{}
	anonymous := make(map[string]interface{})
	err := json.Unmarshal(data, &anonymous)
	if err != nil {
		return nil, err
	}
	for term, keys := range registered {
		for _, key := range keys {
			if _, ok := anonymous[key]; !ok {
				goto nextTerm
			}
		}
		err = json.Unmarshal(data, term)
		if err != nil {
			return nil, err
		}
		if toInit, ok := term.(Initer); ok {
			if err := toInit.Init(); err != nil {
				return term, err
			}
		}
		return term, nil
	nextTerm:
	}
	return nil, fmt.Errorf("parsing into yield curve failed")

}
