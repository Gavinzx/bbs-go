package xlsx

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	rows := [][]string{
		{"用户名", "邮箱", "昵称", "密码"},
		{"zhang_san", "zhang_san@example.com", "张三", "Pass@12345"},
		{"li_si", "li_si@example.com", "李四", ""},
	}

	data, err := Write(rows)
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty xlsx bytes")
	}

	got, err := Read(data)
	if err != nil {
		t.Fatalf("read xlsx: %v", err)
	}
	if len(got) != len(rows) {
		t.Fatalf("expected %d rows, got %d: %#v", len(rows), len(got), got)
	}
	for i := range rows {
		if len(got[i]) != len(rows[i]) {
			t.Fatalf("row %d: expected %d cells, got %d: %#v", i, len(rows[i]), len(got[i]), got[i])
		}
		for j := range rows[i] {
			if got[i][j] != rows[i][j] {
				t.Fatalf("cell %d,%d: expected %q, got %q", i, j, rows[i][j], got[i][j])
			}
		}
	}
}

func TestReadSharedStringsAndSparseCells(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	add("xl/workbook.xml", `<?xml version="1.0"?><workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships><Relationship Id="rId1" Type="worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/sharedStrings.xml", `<?xml version="1.0"?><sst><si><t>用户名</t></si><si><r><t>zhang</t></r><r><t>_san</t></r></si></sst>`)
	add("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1" t="inlineStr"><is><t>昵称</t></is></c></row><row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2"><v>42</v></c></row></sheetData></worksheet>`)

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatalf("read xlsx: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %#v", got)
	}
	if got[0][0] != "用户名" || got[0][2] != "昵称" {
		t.Fatalf("unexpected first row: %#v", got[0])
	}
	if got[1][0] != "zhang_san" || got[1][1] != "42" {
		t.Fatalf("unexpected second row: %#v", got[1])
	}
}

func TestReadRowsPreservesExcelRowNumbersAndInlineRichText(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	add("xl/workbook.xml", `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Type="worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>用户名</t></is></c></row><row r="4"><c r="A4" t="inlineStr"><is><r><t>zhang</t></r><r><t>_san</t></r></is></c></row></sheetData></worksheet>`)

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	rows, err := ReadRows(buf.Bytes())
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %#v", rows)
	}
	if rows[0].Number != 1 || rows[1].Number != 4 {
		t.Fatalf("expected row numbers 1 and 4, got %#v", rows)
	}
	if rows[1].Values[0] != "zhang_san" {
		t.Fatalf("expected inline rich text to be joined, got %#v", rows[1].Values)
	}
}
