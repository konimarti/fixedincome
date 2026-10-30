package source

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// readTable reads a delimited text file: it strips a byte order mark,
// accepts LF and CRLF, skips blank lines and any preamble before the header
// row, i.e. the first row that contains all the columns in need (compared
// case-insensitively; columns are found by name, never by position). Every
// row after the header is passed to row with its (1-based) line number, the
// fields and the column index by lower-case name. The empty file and a file
// without header are errors.
func readTable(r io.Reader, sep byte, need []string, row func(line int, f []string, col map[string]int) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	var col map[string]int
	line, seen := 0, false
	for sc.Scan() {
		line++
		text := sc.Text()
		if line == 1 {
			text = strings.TrimPrefix(text, "\xef\xbb\xbf")
		}
		text = strings.TrimRight(text, "\r")
		if strings.TrimSpace(text) == "" {
			continue
		}
		seen = true
		f := splitFields(text, sep)
		if col == nil {
			names := map[string]int{}
			for i, n := range f {
				n = strings.ToLower(strings.TrimSpace(n))
				if _, dup := names[n]; !dup {
					names[n] = i
				}
			}
			ok := true
			for _, n := range need {
				if _, found := names[strings.ToLower(n)]; !found {
					ok = false
				}
			}
			if ok {
				col = names
			}
			continue
		}
		if err := row(line, f, col); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("line %d is longer than %d bytes", line+1, maxLine)
		}
		return fmt.Errorf("reading: %w", err)
	}
	if col == nil {
		if !seen {
			return errors.New("empty input")
		}
		return fmt.Errorf("no header row with the columns %s", strings.Join(need, ", "))
	}
	return nil
}

// field returns the field of column name, or an error for a short row.
func field(f []string, col map[string]int, name string, line int) (string, error) {
	i := col[name]
	if i >= len(f) {
		return "", fmt.Errorf("line %d: row has %d fields, column %q is missing", line, len(f), name)
	}
	return strings.TrimSpace(f[i]), nil
}
