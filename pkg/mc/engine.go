package mc

import (
	"fmt"
	"math"
)

const (
	IncompleteSetup = iota
	Initialized
	Running
	ResultsAvailable
)

type Model interface {
	Measurement() float64
}

type Engine struct {
	Model     Model
	Nsim      int
	Estimates []float64
	Status    int
}

func New(m Model, nsim int) *Engine {
	e := Engine{
		Model:     m,
		Nsim:      nsim,
		Estimates: make([]float64, nsim),
		Status:    Initialized,
	}
	return &e
}

func (e *Engine) Run() error {
	if e.Status != Initialized {
		return fmt.Errorf("Monte Carlo engine not initialized")
	}
	e.Status = Running
	for i := 0; i < e.Nsim; i += 1 {
		e.Estimates[i] = e.Model.Measurement()
	}
	e.Status = ResultsAvailable
	return nil
}

// Estimate returns the estimate of the simulation
func (e *Engine) Estimate() (float64, error) {
	if e.Status != ResultsAvailable {
		return 0.0, fmt.Errorf("no results available from Monte Carlo simulation")
	}
	var value float64
	for _, estimate := range e.Estimates {
		value += estimate
	}
	return value / float64(len(e.Estimates)), nil
}

// StdError returns the standard error of the MC simulation
func (e *Engine) StdError() (float64, error) {
	if e.Status != ResultsAvailable {
		return 0.0, fmt.Errorf("no results available from Monte Carlo simulation")
	}
	average, err := e.Estimate()
	if err != nil {
		return 0.0, err
	}
	stderror := 0.0
	for _, estimate := range e.Estimates {
		stderror += math.Pow(estimate-average, 2.0)
	}
	size := float64(len(e.Estimates))
	return math.Sqrt(stderror/size) / math.Sqrt(size), nil

}

// CI returns the 95% confidence interval
func (e *Engine) CI() (float64, float64, error) {
	average, err := e.Estimate()
	if err != nil {
		return 0.0, 0.0, err
	}
	stderror, err := e.StdError()
	if err != nil {
		return 0.0, 0.0, err
	}

	return average - 1.96*stderror, average + 1.96*stderror, nil
}
