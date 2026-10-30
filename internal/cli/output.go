package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
)

// format is an output format.
type format int

const (
	formatText format = iota
	formatCSV
	formatJSON
)

// precision is the number of significant digits of csv and json numbers.
const precision = 10

// textLabels renames columns in the text output of single records (the
// labels of the text output are stable and golden-tested).
var textLabels = map[string]string{"ttm_years": "years_to_maturity"}

// isTerminal reports whether w is a terminal: an *os.File that is a
// character device but not /dev/null (other character devices such as
// /dev/zero or /dev/full are treated as terminals; this is not a full
// isatty check, which would need golang.org/x/term or x/sys, not available
// in the module).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(st, null) {
		return false
	}
	return true
}

// resolveFormat returns the output format for the --format value. The
// default (empty) is text on a terminal and csv otherwise for commands that
// print tables (table=true, also when writing to a file), and text for
// commands that print a single record.
func resolveFormat(value string, table bool, stdout io.Writer, toFile bool) (format, error) {
	switch strings.ToLower(value) {
	case "text":
		return formatText, nil
	case "csv":
		return formatCSV, nil
	case "json":
		return formatJSON, nil
	case "":
		if table && (toFile || !isTerminal(stdout)) {
			return formatCSV, nil
		}
		return formatText, nil
	}
	return 0, usagef("invalid --format %q (text, csv, json)", value)
}

// table is a table of values: float64, int, string, bool or nil (empty).
type table struct {
	cols []string
	rows [][]any
}

func (t *table) add(vals ...any) {
	if len(vals) != len(t.cols) {
		panic(fmt.Sprintf("cli: row with %d values for %d columns", len(vals), len(t.cols)))
	}
	t.rows = append(t.rows, vals)
}

// num formats a float for csv and json with full precision (%.10g); negative
// zero is printed as 0.
func num(v float64) string {
	if v == 0 {
		v = 0
	}
	return strconv.FormatFloat(v, 'g', precision, 64)
}

func isNaNInf(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }

// cellText formats a value for csv (empty for nil and non-finite numbers)
// and text output.
func cellText(v any, textual bool) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		if isNaNInf(x) {
			if textual {
				return "n/a"
			}
			return ""
		}
		if textual {
			if x == 0 {
				x = 0
			}
			return strconv.FormatFloat(x, 'g', 8, 64)
		}
		return num(x)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case string:
		return x
	}
	return fmt.Sprint(v)
}

// jsonValue formats a value as a JSON value (null for nil and non-finite
// numbers).
func jsonValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case float64:
		if isNaNInf(x) {
			return "null"
		}
		return num(x)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	}
	b, _ := json.Marshal(fmt.Sprint(v))
	return string(b)
}

// csvGuard prefixes a text field that a spreadsheet could interpret as a
// formula (leading = + - @, tab or carriage return) with an apostrophe.
func csvGuard(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// csvEscape quotes a csv field if needed.
func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\r\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// writeTable writes the table in the given format: csv with header, json as
// an array of objects, text as an aligned table.
func writeTable(w io.Writer, t *table, f format) error {
	bw := bufio.NewWriter(w)
	switch f {
	case formatCSV:
		fmt.Fprintln(bw, strings.Join(t.cols, ","))
		for _, r := range t.rows {
			for i, v := range r {
				if i > 0 {
					bw.WriteByte(',')
				}
				text := cellText(v, false)
				if _, isString := v.(string); isString {
					text = csvGuard(text)
				}
				bw.WriteString(csvEscape(text))
			}
			bw.WriteByte('\n')
		}
	case formatJSON:
		bw.WriteString("[")
		for i, r := range t.rows {
			if i > 0 {
				bw.WriteString(",")
			}
			bw.WriteString("\n  " + jsonObject(t.cols, r))
		}
		bw.WriteString("\n]\n")
	default:
		tw := tabwriter.NewWriter(bw, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join(t.cols, "\t"))
		for _, r := range t.rows {
			cells := make([]string, len(r))
			for i, v := range r {
				cells[i] = cellText(v, true)
			}
			fmt.Fprintln(tw, strings.Join(cells, "\t"))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// jsonObject formats one record as a JSON object with ordered keys.
func jsonObject(cols []string, vals []any) string {
	var sb strings.Builder
	sb.WriteString("{")
	for i, c := range cols {
		if i > 0 {
			sb.WriteString(", ")
		}
		k, _ := json.Marshal(c)
		sb.Write(k)
		sb.WriteString(": ")
		sb.WriteString(jsonValue(vals[i]))
	}
	sb.WriteString("}")
	return sb.String()
}

// writeRecord writes a single record: text as "label: value" lines, csv as
// header and one row, json as one object.
func writeRecord(w io.Writer, cols []string, vals []any, f format) error {
	switch f {
	case formatText:
		bw := bufio.NewWriter(w)
		for i, c := range cols {
			if l, ok := textLabels[c]; ok {
				c = l
			}
			fmt.Fprintf(bw, "%s: %s\n", c, cellText(vals[i], true))
		}
		return bw.Flush()
	case formatJSON:
		_, err := fmt.Fprintln(w, jsonObject(cols, vals))
		return err
	}
	t := &table{cols: cols}
	t.add(vals...)
	return writeTable(w, t, formatCSV)
}

// output is the destination of the data (stdout or a file given by -o). A
// file is written to a temporary file in the same directory and renamed on
// Close, so that a failure never damages an existing file.
type output struct {
	w      io.Writer
	f      *os.File
	toFile bool
	path   string
	// tmp is the temporary file to rename (empty if the file is written
	// directly, e.g. a device)
	tmp string
}

// sameFile reports whether the paths refer to the same file (os.SameFile if
// both exist, else the same absolute path).
func sameFile(a, b string) bool {
	if a == "" || b == "" || a == "-" || b == "-" {
		return false
	}
	if sa, err := os.Stat(a); err == nil {
		if sb, err := os.Stat(b); err == nil {
			return os.SameFile(sa, sb)
		}
	}
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && aa == bb
}

// openOutput opens the output file (before the computation, so that a bad
// path fails early); "" and "-" mean stdout. It is a usage error if the
// output is one of the input files.
func openOutput(path string, stdout io.Writer, inputs ...string) (*output, error) {
	if path == "" || path == "-" {
		return &output{w: stdout}, nil
	}
	for _, in := range inputs {
		if sameFile(path, in) {
			return nil, usagef("the output file %s is also an input file", path)
		}
	}
	if st, err := os.Stat(path); err == nil && !st.Mode().IsRegular() {
		// a device or pipe (e.g. /dev/stdout): write directly
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return nil, fmt.Errorf("creating output file: %w", err)
		}
		return &output{w: f, f: f, toFile: true, path: path}, nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return nil, fmt.Errorf("creating output file: %w", err)
	}
	return &output{w: f, f: f, toFile: true, path: path, tmp: f.Name()}, nil
}

// Close closes the file and moves it to its final name; it returns the
// error of the close or the rename.
func (o *output) Close() error {
	if o.f == nil {
		return nil
	}
	f := o.f
	o.f = nil
	if err := f.Close(); err != nil {
		if o.tmp != "" {
			os.Remove(o.tmp)
		}
		return fmt.Errorf("closing %s: %w", o.path, err)
	}
	if o.tmp != "" {
		if err := os.Chmod(o.tmp, 0o644); err != nil {
			os.Remove(o.tmp)
			return fmt.Errorf("writing %s: %w", o.path, err)
		}
		if err := os.Rename(o.tmp, o.path); err != nil {
			os.Remove(o.tmp)
			return fmt.Errorf("writing %s: %w", o.path, err)
		}
	}
	return nil
}

// Discard closes the file and removes the temporary file after a failure;
// an existing output file is left untouched.
func (o *output) Discard() {
	if o.f != nil {
		o.f.Close()
		if o.tmp != "" {
			os.Remove(o.tmp)
		}
		o.f = nil
	}
}

// finish closes the output after the command ran: on error the temporary
// file is removed, otherwise Close errors are returned.
func (o *output) finish(err error) error {
	if err != nil {
		o.Discard()
		return err
	}
	return o.Close()
}

// openFile opens a file for reading.
func openFile(path string) (*os.File, error) { return os.Open(path) }
