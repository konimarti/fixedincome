package term

import (
	"encoding/json"
	"fmt"
)

var (
	registered = map[Structure][]string{
		&NelsonSiegelSvensson{}: []string{"b0", "b1", "b2", "b3", "t1", "t2", "spread"},
		&Flat{}:                 []string{"r", "spread"},
		&Spline{}:               []string{"spline", "spread"},
	}
)

func Parse(data []byte) (Structure, error) {
	// unmarshal data into map[string]interface{}
	anonymous := make(map[string]interface{})
	err := json.Unmarshal(data, &anonymous)
	if err != nil {
		return nil, err
	}
	for term, keys := range registered {
		for _, key := range keys {
			// fmt.Println("checking", key)
			if _, ok := anonymous[key]; !ok {
				// fmt.Println("key failed", key)
				// outdata, _ := json.MarshalIndent(term, " ", "")
				// fmt.Println(string(outdata))
				goto nextTerm
			}
		}
		// fmt.Printf("unmarshelling data for %T\n", term)
		err = json.Unmarshal(data, term)
		if err != nil {
			return nil, err
		}
		return term, nil
	nextTerm:
		// fmt.Println("nextTerm")
	}
	return nil, fmt.Errorf("parsing into yield curve failed")

}
