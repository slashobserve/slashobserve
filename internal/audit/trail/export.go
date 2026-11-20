package trail

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/filter"
)

const (
	MaxExportRows  = 1_000_000
	FormatNDJSON   = "ndjson"
	FormatCSV      = "csv"
	exportPageRows = filter.MaxRows
)

type Export struct {
	Rows      int
	Truncated bool
}

type rowWriter interface {
	write(Event) error
	flush() error
}

type ndjsonWriter struct {
	enc *json.Encoder
}

func (w ndjsonWriter) write(e Event) error { return w.enc.Encode(e) }

func (w ndjsonWriter) flush() error { return nil }

type csvWriter struct {
	w *csv.Writer
}

func (w csvWriter) write(e Event) error { return w.w.Write(e.csvRecord()) }

func (w csvWriter) flush() error {
	w.w.Flush()
	return w.w.Error()
}

func newRowWriter(format string, out io.Writer) (rowWriter, error) {
	if format == FormatCSV {
		w := csv.NewWriter(out)
		return csvWriter{w: w}, w.Write(csvHeader)
	}
	return ndjsonWriter{enc: json.NewEncoder(out)}, nil
}

type Sink struct {
	Begin func() io.Writer
	Flush func()
}

func (r *Reader) Export(ctx context.Context, p Prepared, format string, limit int, sink Sink) (Export, error) {
	result, err := r.export(ctx, p, format, limit, sink)
	data := map[string]any{"format": format, "row_count": result.Rows, "truncated": result.Truncated}
	e := audit.Event{Action: actionExport, Status: http.StatusOK, Data: data}
	if err != nil {
		e.Outcome, e.Reason = audit.OutcomeFailed, "export_failed"
		if problem, ok := ProblemOf(err); ok {
			e.Status, e.Reason = problem.Status, problem.Code
		}
	}
	r.record(ctx, p, e)
	return result, err
}

func (r *Reader) export(ctx context.Context, p Prepared, format string, limit int, sink Sink) (Export, error) {
	var result Export
	var w rowWriter
	page := p
	for {
		batch, err := r.events(ctx, page, min(exportPageRows, limit-result.Rows))
		if err != nil {
			return result, err
		}
		if w == nil {
			if w, err = newRowWriter(format, sink.Begin()); err != nil {
				return result, err
			}
		}
		for _, e := range batch.Events {
			if err := w.write(e); err != nil {
				return result, err
			}
		}
		result.Rows += len(batch.Events)
		if err := w.flush(); err != nil {
			return result, err
		}
		sink.Flush()
		if batch.NextCursor == "" {
			return result, nil
		}
		if result.Rows >= limit {
			result.Truncated = true
			return result, nil
		}
		last := batch.Events[len(batch.Events)-1]
		page.after = &filter.Cursor{OccurredAt: last.at, ID: last.ID}
	}
}
