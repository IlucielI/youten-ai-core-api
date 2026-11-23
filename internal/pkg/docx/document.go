package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// Document represents an Office Open XML (.docx) document builder compatible with Google Docs and Microsoft Word.
type Document struct {
	bodyBuf bytes.Buffer
}

// New creates a new Document instance.
func New() *Document {
	return &Document{}
}

// escapeXML escapes special XML characters.
func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// AddTitle adds a prominent document title at the top.
func (d *Document) AddTitle(title string) {
	d.bodyBuf.WriteString(`
    <w:p>
      <w:pPr>
        <w:pStyle w:val="Title"/>
        <w:jc w:val="left"/>
        <w:spacing w:before="240" w:after="160"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:b/>
          <w:color w:val="0F172A"/>
          <w:sz w:val="48"/>
          <w:szCs w:val="48"/>
        </w:rPr>
        <w:t>` + escapeXML(title) + `</w:t>
      </w:r>
    </w:p>`)
}

// AddHeading1 adds a Heading 1 paragraph (mapped to Google Docs Outline level 1).
func (d *Document) AddHeading1(heading string) {
	d.bodyBuf.WriteString(`
    <w:p>
      <w:pPr>
        <w:pStyle w:val="Heading1"/>
        <w:spacing w:before="360" w:after="140"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:b/>
          <w:color w:val="1E293B"/>
          <w:sz w:val="32"/>
          <w:szCs w:val="32"/>
        </w:rPr>
        <w:t>` + escapeXML(heading) + `</w:t>
      </w:r>
    </w:p>`)
}

// AddHeading2 adds a Heading 2 paragraph (mapped to Google Docs Outline level 2).
func (d *Document) AddHeading2(heading string) {
	d.bodyBuf.WriteString(`
    <w:p>
      <w:pPr>
        <w:pStyle w:val="Heading2"/>
        <w:spacing w:before="280" w:after="100"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:b/>
          <w:color w:val="334155"/>
          <w:sz w:val="26"/>
          <w:szCs w:val="26"/>
        </w:rPr>
        <w:t>` + escapeXML(heading) + `</w:t>
      </w:r>
    </w:p>`)
}

// AddHeading3 adds a Heading 3 paragraph.
func (d *Document) AddHeading3(heading string) {
	d.bodyBuf.WriteString(`
    <w:p>
      <w:pPr>
        <w:pStyle w:val="Heading3"/>
        <w:spacing w:before="200" w:after="80"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:b/>
          <w:color w:val="475569"/>
          <w:sz w:val="22"/>
          <w:szCs w:val="22"/>
        </w:rPr>
        <w:t>` + escapeXML(heading) + `</w:t>
      </w:r>
    </w:p>`)
}

// AddParagraph adds a standard body paragraph with 1.15 line spacing.
func (d *Document) AddParagraph(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	d.bodyBuf.WriteString(`
    <w:p>
      <w:pPr>
        <w:spacing w:after="120" w:line="276" w:lineRule="auto"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:color w:val="1E293B"/>
          <w:sz w:val="21"/>
          <w:szCs w:val="21"/>
        </w:rPr>
        <w:t xml:space="preserve">` + escapeXML(text) + `</w:t>
      </w:r>
    </w:p>`)
}

// AddBulletItem adds a bullet list item with a bullet symbol and indentation.
func (d *Document) AddBulletItem(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	d.bodyBuf.WriteString(`
    <w:p>
      <w:pPr>
        <w:ind w:left="360" w:hanging="240"/>
        <w:spacing w:after="80" w:line="260" w:lineRule="auto"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:color w:val="6366F1"/>
          <w:sz w:val="21"/>
        </w:rPr>
        <w:t>• </w:t>
      </w:r>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:color w:val="1E293B"/>
          <w:sz w:val="21"/>
          <w:szCs w:val="21"/>
        </w:rPr>
        <w:t xml:space="preserve">` + escapeXML(text) + `</w:t>
      </w:r>
    </w:p>`)
}

// AddCallout adds an executive callout box with a subtle light background.
func (d *Document) AddCallout(title, text string) {
	d.bodyBuf.WriteString(`
    <w:tbl>
      <w:tblPr>
        <w:tblW w:w="5000" w:type="pct"/>
        <w:tblBorders>
          <w:top w:val="none"/>
          <w:left w:val="single" w:sz="24" w:space="0" w:color="6366F1"/>
          <w:bottom w:val="none"/>
          <w:right w:val="none"/>
        </w:tblBorders>
        <w:tblCellMar>
          <w:top w:w="120" w:type="dxa"/>
          <w:left w:w="200" w:type="dxa"/>
          <w:bottom w:w="120" w:type="dxa"/>
          <w:right w:w="200" w:type="dxa"/>
        </w:tblCellMar>
      </w:tblPr>
      <w:tr>
        <w:tc>
          <w:tcPr>
            <w:shd w:val="clear" w:color="auto" w:fill="F8FAFC"/>
          </w:tcPr>
          <w:p>
            <w:pPr>
              <w:spacing w:after="60"/>
            </w:pPr>
            <w:r>
              <w:rPr>
                <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
                <w:b/>
                <w:color w:val="4338CA"/>
                <w:sz w:val="21"/>
              </w:rPr>
              <w:t>` + escapeXML(title) + `</w:t>
            </w:r>
          </w:p>
          <w:p>
            <w:pPr>
              <w:spacing w:after="0" w:line="260" w:lineRule="auto"/>
            </w:pPr>
            <w:r>
              <w:rPr>
                <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
                <w:color w:val="334155"/>
                <w:sz w:val="20"/>
              </w:rPr>
              <w:t xml:space="preserve">` + escapeXML(text) + `</w:t>
            </w:r>
          </w:p>
        </w:tc>
      </w:tr>
    </w:tbl>
    <w:p><w:pPr><w:spacing w:after="160"/></w:pPr></w:p>`)
}

// AddTable adds a styled executive data table with a soft shaded header.
func (d *Document) AddTable(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	d.bodyBuf.WriteString(`
    <w:tbl>
      <w:tblPr>
        <w:tblW w:w="5000" w:type="pct"/>
        <w:tblBorders>
          <w:top w:val="single" w:sz="6" w:space="0" w:color="E2E8F0"/>
          <w:left w:val="single" w:sz="6" w:space="0" w:color="E2E8F0"/>
          <w:bottom w:val="single" w:sz="6" w:space="0" w:color="CBD5E1"/>
          <w:right w:val="single" w:sz="6" w:space="0" w:color="E2E8F0"/>
          <w:insideH w:val="single" w:sz="4" w:space="0" w:color="E2E8F0"/>
          <w:insideV w:val="single" w:sz="4" w:space="0" w:color="E2E8F0"/>
        </w:tblBorders>
        <w:tblCellMar>
          <w:top w:w="120" w:type="dxa"/>
          <w:left w:w="160" w:type="dxa"/>
          <w:bottom w:w="120" w:type="dxa"/>
          <w:right w:w="160" w:type="dxa"/>
        </w:tblCellMar>
      </w:tblPr>`)

	// Header row
	d.bodyBuf.WriteString(`
      <w:tr>
        <w:trPr>
          <w:tblHeader/>
        </w:trPr>`)
	for _, h := range headers {
		d.bodyBuf.WriteString(`
        <w:tc>
          <w:tcPr>
            <w:shd w:val="clear" w:color="auto" w:fill="F1F5F9"/>
          </w:tcPr>
          <w:p>
            <w:pPr>
              <w:spacing w:after="40" w:before="40"/>
            </w:pPr>
            <w:r>
              <w:rPr>
                <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
                <w:b/>
                <w:color w:val="1E293B"/>
                <w:sz w:val="20"/>
              </w:rPr>
              <w:t>` + escapeXML(h) + `</w:t>
            </w:r>
          </w:p>
        </w:tc>`)
	}
	d.bodyBuf.WriteString(`
      </w:tr>`)

	// Data rows
	for _, row := range rows {
		d.bodyBuf.WriteString(`
      <w:tr>`)
		for i, col := range row {
			val := col
			if i >= len(headers) {
				break
			}
			d.bodyBuf.WriteString(`
        <w:tc>
          <w:p>
            <w:pPr>
              <w:spacing w:after="40" w:before="40"/>
            </w:pPr>
            <w:r>
              <w:rPr>
                <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
                <w:color w:val="334155"/>
                <w:sz w:val="19"/>
              </w:rPr>
              <w:t xml:space="preserve">` + escapeXML(val) + `</w:t>
            </w:r>
          </w:p>
        </w:tc>`)
		}
		// Pad missing cells if any
		if len(row) < len(headers) {
			for k := len(row); k < len(headers); k++ {
				d.bodyBuf.WriteString(`
        <w:tc><w:p><w:pPr><w:spacing w:after="40" w:before="40"/></w:pPr></w:p></w:tc>`)
			}
		}
		d.bodyBuf.WriteString(`
      </w:tr>`)
	}

	d.bodyBuf.WriteString(`
    </w:tbl>
    <w:p><w:pPr><w:spacing w:after="160"/></w:pPr></w:p>`)
}

// AddTranscriptTurn adds a speaker dialogue entry with timestamp badge.
func (d *Document) AddTranscriptTurn(speaker, timestamp, text string) {
	d.bodyBuf.WriteString(`
    <w:p>
      <w:pPr>
        <w:spacing w:before="80" w:after="40" w:line="260" w:lineRule="auto"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:b/>
          <w:color w:val="4338CA"/>
          <w:sz w:val="20"/>
        </w:rPr>
        <w:t>` + escapeXML(speaker) + `</w:t>
      </w:r>`)

	if timestamp != "" {
		d.bodyBuf.WriteString(`
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:color w:val="64748B"/>
          <w:sz w:val="18"/>
        </w:rPr>
        <w:t xml:space="preserve">  [` + escapeXML(timestamp) + `]</w:t>
      </w:r>`)
	}

	d.bodyBuf.WriteString(`
    </w:p>
    <w:p>
      <w:pPr>
        <w:ind w:left="280"/>
        <w:spacing w:after="100" w:line="260" w:lineRule="auto"/>
      </w:pPr>
      <w:r>
        <w:rPr>
          <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
          <w:color w:val="1E293B"/>
          <w:sz w:val="20"/>
        </w:rPr>
        <w:t xml:space="preserve">` + escapeXML(text) + `</w:t>
      </w:r>
    </w:p>`)
}

// Bytes packs the constructed XML fragments into a fully-compliant Office Open XML (.docx) ZIP archive.
func (d *Document) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	files := map[string]string{
		"[Content_Types].xml":            contentTypesXML,
		"_rels/.rels":                    relsXML,
		"word/_rels/document.xml.rels":   documentRelsXML,
		"word/styles.xml":                stylesXML,
		"word/settings.xml":              settingsXML,
		"docProps/core.xml":              corePropsXML(time.Now()),
		"word/document.xml":              documentWrapperXML(d.bodyBuf.String()),
	}

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("failed to create zip entry %s: %w", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("failed to write zip entry %s: %w", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize docx zip archive: %w", err)
	}

	return buf.Bytes(), nil
}

// XML Templates

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
  <Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
</Relationships>`

const documentRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>
</Relationships>`

const settingsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:defaultTabStop w:val="720"/>
  <w:characterSpacingControl w:val="doNotCompress"/>
</w:settings>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:docDefaults>
    <w:rPrDefault>
      <w:rPr>
        <w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial"/>
        <w:sz w:val="21"/>
        <w:szCs w:val="21"/>
        <w:color w:val="1E293B"/>
        <w:lang w:val="en-US"/>
      </w:rPr>
    </w:rPrDefault>
    <w:pPrDefault>
      <w:pPr>
        <w:spacing w:after="120" w:line="276" w:lineRule="auto"/>
      </w:pPr>
    </w:pPrDefault>
  </w:docDefaults>

  <w:style w:type="paragraph" w:default="1" w:styleId="Normal">
    <w:name w:val="Normal"/>
    <w:qFormat/>
  </w:style>

  <w:style w:type="paragraph" w:styleId="Title">
    <w:name w:val="Title"/>
    <w:basedOn w:val="Normal"/>
    <w:qFormat/>
    <w:pPr>
      <w:spacing w:before="240" w:after="160"/>
    </w:pPr>
    <w:rPr>
      <w:b/>
      <w:color w:val="0F172A"/>
      <w:sz w:val="48"/>
      <w:szCs w:val="48"/>
    </w:rPr>
  </w:style>

  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/>
    <w:basedOn w:val="Normal"/>
    <w:next w:val="Normal"/>
    <w:uiPriority w:val="9"/>
    <w:qFormat/>
    <w:pPr>
      <w:spacing w:before="360" w:after="140"/>
      <w:outlineLvl w:val="0"/>
    </w:pPr>
    <w:rPr>
      <w:b/>
      <w:color w:val="1E293B"/>
      <w:sz w:val="32"/>
      <w:szCs w:val="32"/>
    </w:rPr>
  </w:style>

  <w:style w:type="paragraph" w:styleId="Heading2">
    <w:name w:val="heading 2"/>
    <w:basedOn w:val="Normal"/>
    <w:next w:val="Normal"/>
    <w:uiPriority w:val="9"/>
    <w:qFormat/>
    <w:pPr>
      <w:spacing w:before="280" w:after="100"/>
      <w:outlineLvl w:val="1"/>
    </w:pPr>
    <w:rPr>
      <w:b/>
      <w:color w:val="334155"/>
      <w:sz w:val="26"/>
      <w:szCs w:val="26"/>
    </w:rPr>
  </w:style>

  <w:style w:type="paragraph" w:styleId="Heading3">
    <w:name w:val="heading 3"/>
    <w:basedOn w:val="Normal"/>
    <w:next w:val="Normal"/>
    <w:uiPriority w:val="9"/>
    <w:qFormat/>
    <w:pPr>
      <w:spacing w:before="200" w:after="80"/>
      <w:outlineLvl w:val="2"/>
    </w:pPr>
    <w:rPr>
      <w:b/>
      <w:color w:val="475569"/>
      <w:sz w:val="22"/>
      <w:szCs w:val="22"/>
    </w:rPr>
  </w:style>
</w:styles>`

func corePropsXML(created time.Time) string {
	ts := created.UTC().Format("2006-01-02T15:04:05Z")
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns:dcterms="http://purl.org/dc/terms/"
  xmlns:dcmitype="http://purl.org/dc/dcmitype/"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>Youten AI Meeting Minutes</dc:title>
  <dc:creator>Youten AI</dc:creator>
  <cp:lastModifiedBy>Youten AI</cp:lastModifiedBy>
  <dcterms:created xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:modified>
</cp:coreProperties>`
}

func documentWrapperXML(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <w:body>` + body + `
    <w:sectPr>
      <w:pgSz w:w="12240" w:h="15840"/>
      <w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/>
      <w:cols w:space="720"/>
      <w:docGrid w:linePitch="360"/>
    </w:sectPr>
  </w:body>
</w:document>`
}
