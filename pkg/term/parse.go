package term

import (
	"encoding/json"
	"fmt"
)

// registeredStructure describes a term structure that can be parsed: the
// factory creates a fresh value and keys are the required JSON fields (the
// "spread" field is optional and defaults to 0)
type registeredStructure struct {
	factory func() Structure
	keys    []string
}

var (
	// registered term structures in the order they are tried (most specific
	// first)
	registered = []registeredStructure{
		{
			factory: func() Structure { return &NelsonSiegelSvensson{} },
			keys:    []string{"b0", "b1", "b2", "b3", "t1", "t2"},
		},
		{
			factory: func() Structure { return &Spline{} },
			keys:    []string{"maturities", "discountfactors"},
		},
		{
			factory: func() Structure { return &Flat{} },
			keys:    []string{"r"},
		},
	}
)

type Initer interface {
	Init() error
}

// Parse returns a new term structure from the JSON data. The type of the
// term structure is determined by the JSON fields. Every call returns a new,
// independent value.
func Parse(data []byte) (Structure, error) {
	// unmarshal data into map[string]interface{}
	anonymous := make(map[string]interface{})
	err := json.Unmarshal(data, &anonymous)
	if err != nil {
		return nil, err
	}
	for _, reg := range registered {
		if !hasKeys(anonymous, reg.keys) {
			continue
		}
		term := reg.factory()
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
	}
	return nil, fmt.Errorf("parsing into yield curve failed")

}

func hasKeys(m map[string]interface{}, keys []string) bool {
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			return false
		}
	}
	return true
}
