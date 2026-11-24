package docx_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"code-base-golang/internal/pkg/docx"
)

func TestDocx_CreateDocument(t *testing.T) {
	doc := docx.New()
	doc.AddTitle("Sprint Retrospective & Planning")
	doc.AddHeading1("Ringkasan Eksekutif")
	doc.AddParagraph("Pertemuan membahas evaluasi sprint 12 dan persiapan sprint 13 dengan penekanan pada reliabilitas sistem.")
	doc.AddCallout("Poin Utama", "Semua target tercapai dengan 0 bug kritis.")
	doc.AddHeading2("Rencana Tindak Lanjut")
	doc.AddBulletItem("Pindahkan server ke cluster baru sebelum akhir bulan.")
	doc.AddBulletItem("Tingkatkan coverage unit test hingga 90%.")

	headers := []string{"Tugas", "PIC", "Status"}
	rows := [][]string{
		{"Deploy Google Calendar", "Bayu", "Selesai"},
		{"Dokumentasi API", "Rifqi", "In Progress"},
	}
	doc.AddTable(headers, rows)

	doc.AddHeading2("Transkrip Percakapan")
	doc.AddTranscriptTurn("Bayu", "00:01:15", "Halo tim, mari kita mulai retrospective hari ini.")
	doc.AddTranscriptTurn("Rifqi", "00:01:25", "Halo Mas Bayu, siap.")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("unexpected error creating docx bytes: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty docx bytes")
	}

	// Verify it is a valid zip archive containing all required OOXML files
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed to parse as zip: %v", err)
	}

	expectedFiles := map[string]bool{
		"[Content_Types].xml":          false,
		"_rels/.rels":                  false,
		"word/_rels/document.xml.rels": false,
		"word/styles.xml":              false,
		"word/settings.xml":            false,
		"docProps/core.xml":            false,
		"word/document.xml":            false,
	}

	var docXMLContent string
	for _, f := range zr.File {
		if _, ok := expectedFiles[f.Name]; ok {
			expectedFiles[f.Name] = true
		}
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open word/document.xml: %v", err)
			}
			content, err := io.ReadAll(rc)
			if err != nil {
				_ = rc.Close()
				t.Fatalf("failed to read word/document.xml: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Fatalf("failed to close word/document.xml: %v", err)
			}
			docXMLContent = string(content)
		}
	}

	for fName, found := range expectedFiles {
		if !found {
			t.Errorf("missing expected OOXML file in zip archive: %s", fName)
		}
	}

	// Verify key contents in document.xml
	if !strings.Contains(docXMLContent, "Sprint Retrospective &amp; Planning") {
		t.Errorf("expected escaped title in document.xml, got: %s", docXMLContent)
	}
	if !strings.Contains(docXMLContent, "w:val=\"Heading1\"") {
		t.Errorf("expected Heading1 style in document.xml")
	}
	if !strings.Contains(docXMLContent, "w:val=\"Heading2\"") {
		t.Errorf("expected Heading2 style in document.xml")
	}
	if !strings.Contains(docXMLContent, "Deploy Google Calendar") {
		t.Errorf("expected table row data in document.xml")
	}
	if !strings.Contains(docXMLContent, "Halo tim, mari kita mulai retrospective hari ini.") {
		t.Errorf("expected transcript dialogue in document.xml")
	}
}

