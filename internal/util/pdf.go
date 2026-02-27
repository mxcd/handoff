package util

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/phpdave11/gofpdf"
)

// ImageToPDF wraps image data (JPEG, PNG) in a single-page PDF sized to the image.
// Returns the PDF bytes and "application/pdf" content type.
func ImageToPDF(data []byte, contentType string) ([]byte, error) {
	// Decode image dimensions
	r := bytes.NewReader(data)
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil, fmt.Errorf("decode image config: %w", err)
	}

	// Convert pixels to mm at 72 DPI (1 inch = 25.4 mm)
	const dpi = 72.0
	wMM := float64(cfg.Width) / dpi * 25.4
	hMM := float64(cfg.Height) / dpi * 25.4

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: wMM, Ht: hMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	// Determine image type for gofpdf registration
	var imgType string
	switch {
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		imgType = "JPEG"
	case strings.Contains(contentType, "png"):
		imgType = "PNG"
	default:
		return nil, fmt.Errorf("unsupported image type for PDF conversion: %s", contentType)
	}

	pdf.RegisterImageOptionsReader("img", gofpdf.ImageOptions{ImageType: imgType}, bytes.NewReader(data))
	pdf.ImageOptions("img", 0, 0, wMM, hMM, false, gofpdf.ImageOptions{}, 0, "")

	if pdf.Err() {
		return nil, fmt.Errorf("generate PDF: %w", pdf.Error())
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// SVGToPDF wraps SVG data in a single-page PDF. Since gofpdf doesn't support
// SVG natively, we render a fixed A4 landscape page with the SVG note.
// For proper SVG embedding, the SVG is first rasterized — but since signature
// SVGs are simple paths, we convert to PNG on the client side when PDF is requested.
// This function handles the case where SVG data arrives anyway.
func SVGToPDF(data []byte) ([]byte, error) {
	// For SVG, create a minimal A4 page. The caller should prefer sending PNG
	// when output_format is PDF, but this provides a fallback.
	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.AddPage()
	pdf.SetFont("Courier", "", 8)
	pdf.MultiCell(0, 4, string(data), "", "", false)

	if pdf.Err() {
		return nil, fmt.Errorf("generate SVG PDF: %w", pdf.Error())
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write SVG PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// ImagesToPDF assembles multiple images into a single multi-page PDF.
// Each image becomes one page sized to its pixel dimensions at 72 DPI.
func ImagesToPDF(pages [][]byte, contentTypes []string) ([]byte, error) {
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages provided")
	}
	if len(pages) != len(contentTypes) {
		return nil, fmt.Errorf("pages and contentTypes length mismatch")
	}

	// Decode first image to set initial page size
	firstCfg, _, err := image.DecodeConfig(bytes.NewReader(pages[0]))
	if err != nil {
		return nil, fmt.Errorf("decode first page config: %w", err)
	}
	const dpi = 72.0
	wMM := float64(firstCfg.Width) / dpi * 25.4
	hMM := float64(firstCfg.Height) / dpi * 25.4

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: wMM, Ht: hMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)

	for i, imgData := range pages {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(imgData))
		if err != nil {
			return nil, fmt.Errorf("decode page %d config: %w", i, err)
		}
		pageW := float64(cfg.Width) / dpi * 25.4
		pageH := float64(cfg.Height) / dpi * 25.4

		// Determine image type
		var imgType string
		ct := contentTypes[i]
		switch {
		case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
			imgType = "JPEG"
		case strings.Contains(ct, "png"):
			imgType = "PNG"
		default:
			return nil, fmt.Errorf("unsupported image type for PDF: %s", ct)
		}

		pdf.AddPageFormat("P", gofpdf.SizeType{Wd: pageW, Ht: pageH})
		imgName := fmt.Sprintf("page_%d", i)
		pdf.RegisterImageOptionsReader(imgName,
			gofpdf.ImageOptions{ImageType: imgType},
			bytes.NewReader(imgData))
		pdf.ImageOptions(imgName, 0, 0, pageW, pageH, false, gofpdf.ImageOptions{}, 0, "")
	}

	if pdf.Err() {
		return nil, fmt.Errorf("gofpdf error: %w", pdf.Error())
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write PDF: %w", err)
	}
	return buf.Bytes(), nil
}
