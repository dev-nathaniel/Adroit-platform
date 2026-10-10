package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// PrintJSON marshals data into indented JSON and writes to stdout.
func PrintJSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// Table displays formatted rows in standard tabular layout.
type Table struct {
	headers []string
	rows    [][]string
}

// NewTable constructs a table with specified column headers.
func NewTable(headers ...string) *Table {
	return &Table{
		headers: headers,
		rows:    make([][]string, 0),
	}
}

// AddRow appends a row of values to the table.
func (t *Table) AddRow(values ...string) {
	t.rows = append(t.rows, values)
}

// Render writes the formatted table to stdout or target writer.
func (t *Table) Render(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}

	tw := tabwriter.NewWriter(w, 0, 4, 3, ' ', 0)

	// Format headers with Lipgloss bold cyan
	styledHeaders := make([]string, len(t.headers))
	for i, h := range t.headers {
		styledHeaders[i] = StylePrimary.Render(strings.ToUpper(h))
	}
	fmt.Fprintln(tw, strings.Join(styledHeaders, "\t"))

	// Render data rows
	for _, row := range t.rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	fmt.Fprintln(w)
	_ = tw.Flush()
	fmt.Fprintln(w)
}
