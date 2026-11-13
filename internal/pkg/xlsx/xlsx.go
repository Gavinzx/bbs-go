// Package xlsx 提供最小可用的 xlsx 读写能力，避免为模板下载和 Excel 导入引入额外依赖。
package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

const (
	contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

	rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

	workbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="用户导入" sheetId="1" r:id="rId1"/></sheets>
</workbook>`

	workbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

	stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>
<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>
<borders count="1"><border/></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>
<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`
)

// Write 生成只含表头的 xlsx 模板。
func Write(rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	add := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}

	if err := add("[Content_Types].xml", contentTypesXML); err != nil {
		return nil, err
	}
	if err := add("_rels/.rels", rootRelsXML); err != nil {
		return nil, err
	}
	if err := add("xl/workbook.xml", workbookXML); err != nil {
		return nil, err
	}
	if err := add("xl/_rels/workbook.xml.rels", workbookRelsXML); err != nil {
		return nil, err
	}
	if err := add("xl/styles.xml", stylesXML); err != nil {
		return nil, err
	}
	if err := add("xl/worksheets/sheet1.xml", buildSheetXML(rows)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildSheetXML(rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	b.WriteString(`<cols><col min="1" max="1" width="22" customWidth="1"/><col min="2" max="2" width="32" customWidth="1"/><col min="3" max="3" width="20" customWidth="1"/><col min="4" max="4" width="22" customWidth="1"/></cols>`)
	b.WriteString(`<sheetData>`)
	for rowIndex, row := range rows {
		b.WriteString(fmt.Sprintf(`<row r="%d">`, rowIndex+1))
		for colIndex, value := range row {
			ref := cellRef(colIndex, rowIndex)
			style := ""
			if rowIndex == 0 {
				style = ` s="1"`
			}
			b.WriteString(fmt.Sprintf(`<c r="%s" t="inlineStr"%s><is><t xml:space="preserve">%s</t></is></c>`, ref, style, escapeXML(value)))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func cellRef(colIndex, rowIndex int) string {
	col := colIndex + 1
	var letters []byte
	for col > 0 {
		col--
		letters = append([]byte{byte('A' + col%26)}, letters...)
		col /= 26
	}
	return fmt.Sprintf("%s%d", string(letters), rowIndex+1)
}

func escapeXML(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

type Row struct {
	Number int
	Values []string
}

// Read 读取 xlsx 第一个工作表，返回按行列展开的文本值。
func Read(data []byte) ([][]string, error) {
	rows, err := ReadRows(data)
	if err != nil {
		return nil, err
	}
	result := make([][]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Values)
	}
	return result, nil
}

// ReadRows 读取 xlsx 第一个工作表，并保留 Excel 中的真实行号。
func ReadRows(data []byte) ([]Row, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("打开 Excel 文件失败: %w", err)
	}

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	shared, err := readSharedStrings(files)
	if err != nil {
		return nil, err
	}

	sheetPath := resolveFirstSheetPath(files)
	if sheetPath == "" {
		return nil, fmt.Errorf("Excel 文件中没有找到工作表")
	}
	sheetFile, ok := files[sheetPath]
	if !ok {
		return nil, fmt.Errorf("Excel 工作表不存在: %s", sheetPath)
	}
	content, err := readZipFile(sheetFile)
	if err != nil {
		return nil, err
	}

	var sheet struct {
		Rows []struct {
			R     int `xml:"r,attr"`
			Cells []struct {
				Ref  string    `xml:"r,attr"`
				Type string    `xml:"t,attr"`
				V    string    `xml:"v"`
				IS   *richText `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(content, &sheet); err != nil {
		return nil, fmt.Errorf("解析 Excel 工作表失败: %w", err)
	}

	result := make([]Row, 0, len(sheet.Rows))
	lastRowNumber := 0
	for _, row := range sheet.Rows {
		rowNumber := row.R
		if rowNumber <= 0 {
			rowNumber = lastRowNumber + 1
		}
		lastRowNumber = rowNumber
		values := make([]string, 0, len(row.Cells))
		for i, cell := range row.Cells {
			col := i
			if cell.Ref != "" {
				if parsed := colIndex(cell.Ref); parsed >= 0 {
					col = parsed
				}
			}
			for len(values) <= col {
				values = append(values, "")
			}
			values[col] = cellValue(cell.Type, cell.V, cell.IS, shared)
		}
		result = append(result, Row{Number: rowNumber, Values: values})
	}
	return result, nil
}

type richText struct {
	T    string `xml:"t"`
	Runs []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (r *richText) text() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(r.T)
	for _, run := range r.Runs {
		b.WriteString(run.T)
	}
	return b.String()
}

func cellValue(cellType, value string, inline *richText, shared []string) string {
	switch cellType {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && idx >= 0 && idx < len(shared) {
			return shared[idx]
		}
		return ""
	case "inlineStr":
		return inline.text()
	case "b":
		if strings.TrimSpace(value) == "1" {
			return "TRUE"
		}
		return "FALSE"
	default:
		return value
	}
}

func readSharedStrings(files map[string]*zip.File) ([]string, error) {
	file, ok := files["xl/sharedStrings.xml"]
	if !ok {
		return nil, nil
	}
	content, err := readZipFile(file)
	if err != nil {
		return nil, err
	}
	var sst struct {
		Items []richText `xml:"si"`
	}
	if err := xml.Unmarshal(content, &sst); err != nil {
		return nil, fmt.Errorf("解析 Excel 共享字符串失败: %w", err)
	}
	result := make([]string, 0, len(sst.Items))
	for i := range sst.Items {
		result = append(result, sst.Items[i].text())
	}
	return result, nil
}

func resolveFirstSheetPath(files map[string]*zip.File) string {
	if workbook, ok := files["xl/workbook.xml"]; ok {
		if content, err := readZipFile(workbook); err == nil {
			var book struct {
				Sheets []struct {
					RID string `xml:"id,attr"`
				} `xml:"sheets>sheet"`
			}
			if xml.Unmarshal(content, &book) == nil && len(book.Sheets) > 0 {
				rid := book.Sheets[0].RID
				if rels, ok := files["xl/_rels/workbook.xml.rels"]; ok {
					if relContent, err := readZipFile(rels); err == nil {
						var relationships struct {
							Items []struct {
								ID     string `xml:"Id,attr"`
								Target string `xml:"Target,attr"`
							} `xml:"Relationship"`
						}
						if xml.Unmarshal(relContent, &relationships) == nil {
							for _, rel := range relationships.Items {
								if rel.ID != rid {
									continue
								}
								target := strings.TrimPrefix(rel.Target, "/")
								if !strings.HasPrefix(target, "xl/") {
									target = path.Join("xl", target)
								}
								if _, ok := files[target]; ok {
									return target
								}
							}
						}
					}
				}
			}
		}
	}

	if _, ok := files["xl/worksheets/sheet1.xml"]; ok {
		return "xl/worksheets/sheet1.xml"
	}
	for name := range files {
		if strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml") {
			return name
		}
	}
	return ""
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 32<<20))
}

func colIndex(ref string) int {
	col := 0
	for i := 0; i < len(ref); i++ {
		ch := ref[i]
		if ch >= 'A' && ch <= 'Z' {
			col = col*26 + int(ch-'A'+1)
		} else if ch >= 'a' && ch <= 'z' {
			col = col*26 + int(ch-'a'+1)
		} else {
			break
		}
	}
	if col == 0 {
		return -1
	}
	return col - 1
}
