package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	xdraw "golang.org/x/image/draw"
)

const (
	dpi    = 300.0
	bleed  = 0.125
	margin = 0.125

	stdWidth  = 2.5
	stdHeight = 3.5

	miniWidth  = 1.625 // US Mini (Mini American)
	miniHeight = 2.5

	srcDir         = "assets/qrcodes"
	outDirStandard = "assets/cards/back/standard"
	outDirMini     = "assets/cards/back/usmini"
)

var (
	cardBackgroundColor = color.RGBA{0, 0, 0, 255}
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".png") {
			continue
		}

		if err := processFile(entry.Name()); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to process %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}

func processFile(filename string) error {
	srcPath := filepath.Join(srcDir, filename)
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	srcImg, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("failed to decode png: %w", err)
	}

	baseName := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Generate Standard Card
	cardFilename := fmt.Sprintf("%s.png", baseName)
	outfilePathStandard := filepath.Join(outDirStandard, cardFilename)
	if _, err := os.Stat(outfilePathStandard); err == nil {
		return nil
	}

	fmt.Printf("Processing %s...\n", baseName)

	if err := generateCard(srcImg, stdWidth, stdHeight, outfilePathStandard); err != nil {
		return fmt.Errorf("failed to generate standard card: %w", err)
	}

	// Generate Mini Card
	if err := generateCard(srcImg, miniWidth, miniHeight, filepath.Join(outDirMini, cardFilename)); err != nil {
		return fmt.Errorf("failed to generate mini card: %w", err)
	}

	return nil
}

func generateCard(qrImg image.Image, widthIn, heightIn float64, outPath string) error {
	// Calculate dimensions in pixels
	// Total size = (Width + 2*Bleed) * DPI
	totalWidth := int((widthIn + 2*bleed) * dpi)
	totalHeight := int((heightIn + 2*bleed) * dpi)

	// Create canvas
	dst := image.NewRGBA(image.Rect(0, 0, totalWidth, totalHeight))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{cardBackgroundColor}, image.Point{}, draw.Src)

	// Calculate safe area for QR code
	// Safe width = (Width - 2*Margin) * DPI
	// We want the QR code to extend to the margin.
	// The margin is measured from the TRIM line (which is Bleed pixels in).
	// So the box is:
	//   Left: (Bleed + Margin) * DPI
	//   Top:  (Bleed + Margin) * DPI
	//   Width: (Width - 2*Margin) * DPI
	//   Height: (Height - 2*Margin) * DPI (but QR is square, so limited by width)

	// Note: If Height < Width (landscape), it would be limited by height, but cards are portrait.
	// Actually, let's just calculate the max square that fits inside the margins.
	safeW := (widthIn - 2*margin) * dpi
	safeH := (heightIn - 2*margin) * dpi
	qrSize := int(safeW)
	if safeH < safeW {
		qrSize = int(safeH)
	}

	// Calculate position to center the QR code
	// Center of canvas:
	cX := totalWidth / 2
	cY := totalHeight / 2

	// Top-Left of QR code
	qrX := cX - qrSize/2
	qrY := cY - qrSize/2

	qrRect := image.Rect(qrX, qrY, qrX+qrSize, qrY+qrSize)

	// Resize and draw QR code
	// Use CatmullRom for high quality downscaling/upscaling
	xdraw.CatmullRom.Scale(dst, qrRect, qrImg, qrImg.Bounds(), draw.Over, nil)

	// Add Text
	dc := gg.NewContextForRGBA(dst)

	fontPath := "assets/fonts/Lobster-Regular.ttf"
	// Calculate font size relative to width (approx 12% of width)
	fontSize := float64(totalWidth) * 0.12
	if err := dc.LoadFontFace(fontPath, fontSize); err != nil {
		return fmt.Errorf("failed to load font: %w", err)
	}

	dc.SetColor(color.White)
	text := "Temporalize"

	// Top Text
	// Position in the center of the top margin
	topTextY := float64(qrY) / 2.0
	dc.DrawStringAnchored(text, float64(cX), topTextY, 0.5, 0.5)

	// Bottom Text (Upside Down)
	// Position in the center of the bottom margin
	bottomTextY := float64(qrY+qrSize+totalHeight) / 2.0

	dc.Push()
	dc.RotateAbout(gg.Radians(180), float64(cX), bottomTextY)
	dc.DrawStringAnchored(text, float64(cX), bottomTextY, 0.5, 0.5)
	dc.Pop()

	// Save
	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	if err := png.Encode(outFile, dst); err != nil {
		return err
	}

	return nil
}
