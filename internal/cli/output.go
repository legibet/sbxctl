package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

type column struct {
	header string
	right  bool
}

type table struct {
	w       io.Writer
	columns []column
	rows    [][]string
}

func newTable(w io.Writer, columns ...column) *table {
	return &table{w: w, columns: columns}
}

func left(header string) column {
	return column{header: header}
}

func right(header string) column {
	return column{header: header, right: true}
}

func (t *table) Row(values ...string) {
	t.rows = append(t.rows, values)
}

func (t *table) Flush() error {
	widths := make([]int, len(t.columns))
	hasHeader := false
	for index, column := range t.columns {
		widths[index] = ansi.StringWidth(column.header)
		hasHeader = hasHeader || column.header != ""
	}
	for _, row := range t.rows {
		for index := range t.columns {
			if index < len(row) {
				widths[index] = max(widths[index], ansi.StringWidth(row[index]))
			}
		}
	}
	if hasHeader {
		if err := t.writeRow(columnHeaders(t.columns), widths); err != nil {
			return err
		}
	}
	for _, row := range t.rows {
		if err := t.writeRow(row, widths); err != nil {
			return err
		}
	}
	return nil
}

func (t *table) Blank() error {
	_, err := fmt.Fprintln(t.w)
	return err
}

func (t *table) writeRow(row []string, widths []int) error {
	for index, column := range t.columns {
		value := ""
		if index < len(row) {
			value = row[index]
		}
		padding := widths[index] - ansi.StringWidth(value)
		if column.right && padding > 0 {
			if _, err := io.WriteString(t.w, strings.Repeat(" ", padding)); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(t.w, value); err != nil {
			return err
		}
		if index < len(t.columns)-1 {
			if !column.right && padding > 0 {
				if _, err := io.WriteString(t.w, strings.Repeat(" ", padding)); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(t.w, "  "); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(t.w, "\n")
	return err
}

func columnHeaders(columns []column) []string {
	headers := make([]string, len(columns))
	for index, column := range columns {
		headers[index] = column.header
	}
	return headers
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func colorsEnabled(flags *rootFlags) bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	return stdoutIsTerminal() && !flags.NoColor && !noColor
}
