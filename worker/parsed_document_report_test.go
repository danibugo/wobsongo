package worker

import (
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
)

// Run explicitly with:
// go test ./worker -run '^TestGenerateParsedDocumentReport$' -count=1 -v
func TestGenerateParsedDocumentReport(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "parsed_document.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, err := external.ParseRaw(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	kept, dropped := filterNoiseChunks(parsed.Chunks)
	rows := make([]reportRow, 0, len(parsed.Chunks))
	keptIndex := 0
	for i, chunk := range parsed.Chunks {
		row := reportRow{Index: i, Page: chunk.Page, LayoutType: string(chunk.LayoutType), Text: chunk.Text, BoundingBox: chunk.BoundingBox}
		if noiseLayoutTypes[chunk.LayoutType] {
			row.Status, row.StatusClass, row.Reason = "DROP", "drop", "layout type is configured as noise"
		} else {
			row.Status, row.StatusClass, row.KeptIndex = "KEEP", "keep", keptIndex
			keptIndex++
		}
		rows = append(rows, row)
	}

	file, err := os.Create(filepath.Join("testdata", "parsed_document_report.html"))
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	defer file.Close()
	data := reportData{Title: parsed.Title, Pages: parsed.PageCount, Total: len(parsed.Chunks), Kept: len(kept), Dropped: dropped, Rows: rows}
	if err := parsedDocumentReportTemplate.Execute(file, data); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("wrote report: %d chunks, %d kept, %d dropped", data.Total, data.Kept, data.Dropped)
}

type reportData struct {
	Title                       string
	Pages, Total, Kept, Dropped int
	Rows                        []reportRow
}
type reportRow struct {
	Index, KeptIndex, Page      int
	LayoutType, Text            string
	BoundingBox                 model.BoundingBox
	Status, StatusClass, Reason string
}

var parsedDocumentReportTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>{{.Title}} — chunk report</title>
<style>
body{font:15px system-ui,sans-serif;margin:32px;background:#f4f6f8;color:#20242a}
h1{margin-bottom:4px}.summary{color:#59636e;margin-bottom:24px}
.chunk{background:white;border:1px solid #d8dee5;border-left:6px solid #7b8794;border-radius:6px;margin:12px 0;padding:14px 18px;white-space:pre-wrap}
.chunk.keep{border-left-color:#16803c}.chunk.drop{border-left-color:#c9362b;background:#fff7f6}
.meta{font-family:ui-monospace,monospace;color:#59636e;font-size:13px;margin-bottom:8px}
.status{font-weight:700}.keep .status{color:#16803c}.drop .status{color:#c9362b}.reason{color:#c9362b;font-style:italic}
</style></head><body>
<h1>{{.Title}}</h1><div class="summary">{{.Pages}} pages · {{.Total}} chunks · <b>{{.Kept}} kept</b> · <b>{{.Dropped}} dropped</b></div>
{{range .Rows}}<article class="chunk {{.StatusClass}}"><div class="meta"><span class="status">{{.Status}}</span> · original #{{.Index}} {{if eq .Status "KEEP"}}· kept #{{.KeptIndex}}{{end}} · page {{.Page}} · layout <b>{{.LayoutType}}</b> · bbox {{.BoundingBox}}</div>{{if .Reason}}<div class="reason">{{.Reason}}</div>{{end}}<div>{{.Text}}</div></article>{{end}}
</body></html>`))
