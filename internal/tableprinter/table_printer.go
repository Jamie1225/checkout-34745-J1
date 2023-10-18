package tableprinter

import (
	"io"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

type TablePrinter struct {
	tableprinter.TablePrinter
	isTTY bool
	cs    *iostreams.ColorScheme
}

// IsTTY gets wether the TablePrinter will render to a terminal.
func (t *TablePrinter) IsTTY() bool {
	return t.isTTY
}

// AddTimeField in TTY mode displays the fuzzy time difference between now and t.
// In non-TTY mode it just displays t with the time.RFC3339 format.
func (tp *TablePrinter) AddTimeField(now, t time.Time, c func(string) string) {
	var tf string
	if tp.isTTY {
		tf = text.FuzzyAgo(now, t)
	} else {
		tf = t.Format(time.RFC3339)
	}
	tp.AddField(tf, tableprinter.WithColor(c))
}

var (
	WithTruncate = tableprinter.WithTruncate
	WithColor    = tableprinter.WithColor
)

type headerOption struct {
	columns []string
}

// New creates a TablePrinter from an IOStreams.
func New(ios *iostreams.IOStreams, headers headerOption) *TablePrinter {
	maxWidth := 80
	isTTY := ios.IsStdoutTTY()
	if isTTY {
		maxWidth = ios.TerminalWidth()
	}

	return NewWithWriter(ios.Out, isTTY, maxWidth, ios.ColorScheme(), headers)
}

// NewWithWriter creates a TablePrinter from a Writer, whether the output is a terminal, the terminal width, and more.
func NewWithWriter(w io.Writer, isTTY bool, maxWidth int, cs *iostreams.ColorScheme, headers headerOption) *TablePrinter {
	tp := &TablePrinter{
		TablePrinter: tableprinter.New(w, isTTY, maxWidth),
		isTTY:        isTTY,
		cs:           cs,
	}

	if isTTY && len(headers.columns) > 0 {
		// Make sure all headers are uppercase.
		for i := range headers.columns {
			// TODO: Consider truncating longer headers e.g., NUMBER, or removing unnecessary headers e.g., DESCRIPTION with no descriptions.
			headers.columns[i] = strings.ToUpper(headers.columns[i])
		}

		// Make sure all header columns are padded - even the last one - to apply the proper TableHeader style.
		// Checking cs.Enabled() avoids having to do that for nearly all CLI tests.
		var paddingFunc func(int, string) string
		if cs.Enabled() {
			paddingFunc = func(width int, text string) string {
				if l := len(text); l < width {
					return text + strings.Repeat(" ", width-l)
				}
				return text
			}
		}

		tp.AddHeader(
			headers.columns,
			tableprinter.WithPadding(paddingFunc),
			tableprinter.WithColor(cs.TableHeader),
		)
	}

	return tp
}

// WithHeader defines the column names for a table.
// Panics if columns is nil or empty.
func WithHeader(columns ...string) headerOption {
	if len(columns) == 0 {
		panic("must define header columns")
	}
	return headerOption{columns}
}

// NoHeader disable printing or checking for a table header.
//
// Deprecated: use WithHeader unless required otherwise.
var NoHeader = headerOption{}

// TruncateNonURL truncates any text that does not begin with "https://" or "http://".
// This is provided for backward compatibility with the old table printer for existing tables.
func TruncateNonURL(maxWidth int, s string) string {
	if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
		return s
	}
	return text.Truncate(maxWidth, s)
}
