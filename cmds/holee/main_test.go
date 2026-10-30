package main

import (
	"bytes"
	"encoding/csv"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"-n", "20", "-m", "5", "-s", "0", "-paths", "10", "-seed", "1"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	header := strings.Join(rows[0], ",")
	if header != "t,zero_rate,fwd_rate,theta,exp_short_rate,discount,mc_discount,mc_stderr,mc_zero_rate" {
		t.Errorf("header %q", header)
	}
	if len(rows) != 22 {
		t.Fatalf("%d rows, want header + 21 nodes", len(rows))
	}
	last := rows[21]
	if last[0] != "5" {
		t.Errorf("last node t=%v, want 5 (the horizon)", last[0])
	}
	for _, r := range rows[1:] {
		if len(r) != len(rows[0]) {
			t.Fatalf("row %v has %d columns", r, len(r))
		}
		z, _ := strconv.ParseFloat(r[5], 64)
		m, _ := strconv.ParseFloat(r[6], 64)
		if math.Abs(z-m) > 1e-9 {
			t.Errorf("σ=0: mc_discount %v, discount %v", m, z)
		}
	}
	if !strings.Contains(errOut.String(), "σ=0") {
		t.Errorf("summary: %q", errOut.String())
	}
	for _, args := range [][]string{{"-n", "0"}, {"-m", "-1"}, {"-s", "-0.01"}, {"-paths", "1"}, {"-f", "missing.json"}, {"extra"}} {
		if err := run(args, &out, &errOut); err == nil {
			t.Errorf("run(%v): expected error", args)
		}
	}
}

// TestRunOutputFirst: the output file is created before the simulation, so
// an invalid -o fails immediately (2^40 paths would take forever).
func TestRunOutputFirst(t *testing.T) {
	var out, errOut bytes.Buffer
	err := run([]string{"-paths", "1099511627776", "-o", t.TempDir() + "/missing/dir/holee.csv"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for an unwritable output file")
	}
}

func TestRunFile(t *testing.T) {
	var out, errOut bytes.Buffer
	name := t.TempDir() + "/holee.csv"
	if err := run([]string{"-n", "4", "-m", "1", "-paths", "100", "-seed", "1", "-o", name}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil || !strings.HasPrefix(string(data), "t,zero_rate,") || out.Len() != 0 {
		t.Errorf("file %q (%v), stdout %q", data, err, out.String())
	}
}

// TestRunWarnings: degenerate inputs give warnings instead of absurd
// dev/SE ratios.
func TestRunWarnings(t *testing.T) {
	for _, c := range []struct {
		args []string
		warn string
	}{
		{[]string{"-s", "0.5", "-m", "30", "-n", "30", "-paths", "1000", "-seed", "7"}, "underflow"},
		{[]string{"-m", "1e-9", "-n", "3", "-paths", "100", "-seed", "7"}, "rounding level"},
	} {
		var out, errOut bytes.Buffer
		if err := run(c.args, &out, &errOut); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(errOut.String(), c.warn) {
			t.Errorf("%v: no %q warning in %q", c.args, c.warn, errOut.String())
		}
		if strings.Contains(errOut.String(), "e+") && !strings.Contains(errOut.String(), "1000 standard errors") {
			t.Errorf("%v: huge dev/SE without warning: %q", c.args, errOut.String())
		}
	}
}
