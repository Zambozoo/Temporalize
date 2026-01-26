package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"temporalize/internal/models"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/skip2/go-qrcode"
	xdraw "golang.org/x/image/draw"
)

const (
	outDirStdFrontName  = "cards/front/standard"
	outDirStdBackName   = "cards/back/standard"
	outDirMiniFrontName = "cards/front/usmini"
	outDirMiniBackName  = "cards/back/usmini"

	fontPathBold    = "assets/fonts/Lobster-Regular.ttf"
	fontPathRegular = "assets/fonts/Arial.ttf"

	iconArtist = "assets/icons/artistIcon.png"
	iconSong   = "assets/icons/songIcon.png"

	dpi    = 300.0
	bleed  = 0.125
	margin = 0.125

	stdWidth   = 2.5
	stdHeight  = 3.5
	miniWidth  = 1.625
	miniHeight = 2.5

	baseFontSize    = 30.0
	lineSpacing     = 1.1
	borderThickness = 0.06
	cornerRadius    = 0.125
)

var (
	cardBackgroundColor = color.RGBA{0, 0, 0, 255}

	DarkRed  = color.RGBA{139, 0, 0, 255}
	LightRed = color.RGBA{255, 160, 122, 255}

	LightBlue = color.RGBA{173, 216, 230, 255}
	DarkBlue  = color.RGBA{0, 0, 139, 255}

	LightYellow = color.RGBA{255, 255, 153, 255}
	DarkYellow  = color.RGBA{184, 134, 11, 255}

	LightGreen = color.RGBA{144, 238, 144, 255}
	DarkGreen  = color.RGBA{0, 100, 0, 255}

	LightGray = color.RGBA{211, 211, 211, 255}
	DarkGray  = color.RGBA{64, 64, 64, 255}

	LightPurple = color.RGBA{192, 128, 192, 255}
	DarkPurple  = color.RGBA{80, 0, 80, 255}

	LightPink = color.RGBA{255, 192, 203, 255}
	DarkPink  = color.RGBA{255, 105, 180, 255}

	Black = color.RGBA{0, 0, 0, 255}
)

type GenreTheme struct {
	Light color.Color
	Dark  color.Color
	Icon  string
}

var genreThemes = map[string]GenreTheme{
	"country": {Light: LightYellow, Dark: DarkYellow, Icon: "assets/icons/countryIcon.jpg"},
	"pop":     {Light: LightPink, Dark: DarkPink, Icon: "assets/icons/popIcon.jpg"},
	"funk":    {Light: LightPurple, Dark: DarkPurple, Icon: "assets/icons/funkIcon.jpg"},
	"hip-hop": {Light: LightRed, Dark: DarkRed, Icon: "assets/icons/hiphopIcon.jpg"},
	"rock":    {Light: LightBlue, Dark: DarkBlue, Icon: "assets/icons/rockIcon.jpg"},
	"default": {Light: LightGray, Dark: DarkGray, Icon: ""},
}

func createQRCodeImage(s *models.Song) (image.Image, error) {
	var amzAlb, amzTrk, appAlb, appTrk string
	if s.AmazonMusic != "" {
		parts := strings.Split(s.AmazonMusic, ":")
		if len(parts) >= 1 {
			amzAlb = parts[0]
		}
		if len(parts) >= 2 {
			amzTrk = parts[1]
		}
	}
	if s.AppleMusic != "" {
		parts := strings.Split(s.AppleMusic, ":")
		if len(parts) >= 1 {
			appAlb = parts[0]
		}
		if len(parts) >= 2 {
			appTrk = parts[1]
		}
	}

	qrBytes, err := compress(s.Explicit, amzAlb, amzTrk, appAlb, appTrk, s.Spotify, s.YoutubeMusic)
	if err != nil {
		return nil, fmt.Errorf("failed to compress qr data: %w", err)
	}

	pngBytes, err := qrcode.Encode(string(qrBytes), qrcode.Low, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to encode qr code: %w", err)
	}

	return png.Decode(bytes.NewReader(pngBytes))
}

func generateCardFront(s *models.Song, outputDir string) error {
	stdDir := filepath.Join(outputDir, outDirStdFrontName)
	miniDir := filepath.Join(outputDir, outDirMiniFrontName)

	if err := os.MkdirAll(stdDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(miniDir, 0755); err != nil {
		return err
	}

	if err := drawFront(s, stdWidth, stdHeight, stdDir); err != nil {
		return err
	}
	return drawFront(s, miniWidth, miniHeight, miniDir)
}

func drawFront(s *models.Song, widthIn, heightIn float64, outDir string) error {
	totalWidth := int((widthIn + 2*bleed) * dpi)
	totalHeight := int((heightIn + 2*bleed) * dpi)

	theme, ok := genreThemes[strings.ToLower(s.Genre)]
	if !ok {
		found := false
		for k, v := range genreThemes {
			if strings.Contains(strings.ToLower(s.Genre), k) {
				theme = v
				found = true
				break
			}
		}
		if !found {
			theme = genreThemes["default"]
		}
	}

	dc := gg.NewContext(totalWidth, totalHeight)
	dc.SetColor(theme.Dark)
	dc.Clear()

	thumbPath := filepath.Join("thumbnails", s.FileName()+".jpeg")
	img, err := gg.LoadImage(thumbPath)
	if err != nil {
		return fmt.Errorf("failed to load thumbnail %s: %w", thumbPath, err)
	}

	safeX := (bleed + margin) * dpi
	safeY := (bleed + margin) * dpi
	safeW := (widthIn - 2*margin) * dpi
	safeH := (heightIn - 2*margin) * dpi

	borderPx := borderThickness * dpi
	effRadius := cornerRadius
	if widthIn >= stdWidth {
		effRadius = 0.165
	}
	radiusPx := effRadius * dpi
	iconColWidth := int(radiusPx * 2)

	scaleFactor := widthIn / miniWidth
	scaledFontSize := baseFontSize * scaleFactor
	yearFontSize := scaledFontSize * 3.5
	textFontSize := scaledFontSize * 1.0

	// headerH := float64(yearFontSize) * 1.2
	// footerH := headerH
	// textRowH := float64(textFontSize) * 1.5

	minHeaderH := float64(yearFontSize) * 1.5
	minFooterH := float64(textFontSize) * 3.5
	availHForArt := safeH - (minHeaderH + minFooterH)

	artSize := safeW
	if availHForArt < artSize {
		artSize = availHForArt
	}
	if artSize < 0 {
		artSize = 0
	}

	artTopY := (float64(totalHeight) - artSize) / 2
	artBottomY := artTopY + artSize
	headerY := (safeY + artTopY) / 2
	safeBottomY := safeY + safeH
	footerY := (artBottomY + safeBottomY) / 2

	artX := (float64(totalWidth) - artSize) / 2
	dc.SetColor(theme.Light)
	dc.DrawRoundedRectangle(artX, artTopY, artSize, artSize, radiusPx)
	dc.Fill()

	innerArtSize := artSize - 2*borderPx
	if innerArtSize > 0 {
		dc.Push()
		innerRadius := radiusPx - borderPx
		if innerRadius < 0 {
			innerRadius = 0
		}
		dc.DrawRoundedRectangle(artX+borderPx, artTopY+borderPx, innerArtSize, innerArtSize, innerRadius)
		dc.Clip()
		resizedArt := resizeImage(img, int(innerArtSize), int(innerArtSize))
		dc.DrawImageAnchored(resizedArt, int(artX+artSize/2), int(artTopY+artSize/2), 0.5, 0.5)
		dc.ResetClip()
		dc.Pop()
	}

	fntBold, err := loadFont(fontPathBold)
	if err != nil {
		return err
	}
	faceYear := truetype.NewFace(fntBold, &truetype.Options{Size: yearFontSize})
	dc.SetFontFace(faceYear)
	dc.SetColor(theme.Light)
	yearStr := fmt.Sprintf("%d", s.Year)
	centerX := float64(totalWidth) / 2
	yearTextNudge := yearFontSize * 0.1
	dc.DrawStringAnchored(yearStr, centerX, headerY-yearTextNudge, 0.5, 0.5)

	genreIconSize := int(yearFontSize * 0.85)
	if theme.Icon != "" {
		imgGenre, err := loadAndProcessIcon(theme.Icon, genreIconSize, theme.Light)
		if err == nil {
			dc.DrawImageAnchored(imgGenre, int(safeX+float64(iconColWidth)/2), int(headerY), 0.5, 0.5)
		}
	}

	if s.Explicit {
		explicitPath := "assets/icons/explicit.png"
		explicitImg, err := gg.LoadImage(explicitPath)
		if err == nil {
			targetW := int(yearFontSize * 0.85)
			bounds := explicitImg.Bounds()
			ratio := float64(bounds.Dx()) / float64(bounds.Dy())
			targetH := int(float64(targetW) / ratio)
			resizedExplicit := resizeImage(explicitImg, targetW, targetH)
			topRightCenterX := safeX + safeW - float64(iconColWidth)/2
			dc.DrawImageAnchored(resizedExplicit, int(topRightCenterX), int(headerY), 0.5, 0.5)
		}
	}

	fntRegular, err := loadFont(fontPathRegular)
	if err != nil {
		return err
	}
	faceText := truetype.NewFace(fntRegular, &truetype.Options{Size: textFontSize})
	dc.SetFontFace(faceText)
	dc.SetColor(theme.Light)

	titleTextNudge := textFontSize * 0.1

	drawTextRow := func(text string, iconPath string, yPos float64) float64 {
		iconImg, err := loadAndProcessIcon(iconPath, int(textFontSize), theme.Light)
		if err != nil {
			log.Printf("Failed to load icon %s: %v", iconPath, err)
			return 0
		}
		iconW := float64(iconImg.Bounds().Dx())
		gap := textFontSize * 0.5
		maxTextW := safeW - (iconW + gap)
		lines := dc.WordWrap(text, maxTextW)
		lineH := textFontSize * lineSpacing
		textBlockH := float64(len(lines)) * lineH
		maxLineW := 0.0
		for _, line := range lines {
			w, _ := dc.MeasureString(line)
			if w > maxLineW {
				maxLineW = w
			}
		}
		totalRowW := iconW + gap + maxLineW
		startX := (float64(totalWidth) - totalRowW) / 2
		dc.DrawImageAnchored(iconImg, int(startX+iconW/2), int(yPos), 0.5, 0.5)
		firstLineY := yPos - textBlockH/2 + lineH/2
		textCenterX := startX + iconW + gap + maxLineW/2
		for i, line := range lines {
			lineY := firstLineY + float64(i)*lineH
			dc.DrawStringAnchored(line, textCenterX, lineY-titleTextNudge, 0.5, 0.5)
		}
		return textBlockH
	}

	measureHeight := func(text string, iconSize float64) float64 {
		maxTextW := safeW - (iconSize + textFontSize*0.5)
		lines := dc.WordWrap(text, maxTextW)
		return float64(len(lines)) * textFontSize * lineSpacing
	}

	titleH := measureHeight(s.Title, textFontSize)
	artistH := measureHeight(strings.Join(s.Artists, ", "), textFontSize)
	blockGap := textFontSize * 0.5
	totalFooterContentH := titleH + blockGap + artistH
	footerContentTopY := footerY - totalFooterContentH/2
	titleCenterY := footerContentTopY + titleH/2
	artistCenterY := footerContentTopY + titleH + blockGap + artistH/2

	drawTextRow(s.Title, iconSong, titleCenterY)
	drawTextRow(strings.Join(s.Artists, ", "), iconArtist, artistCenterY)

	outFileName := fmt.Sprintf("%s-%s.png", s.FileName(), s.Genre)
	outPath := filepath.Join(outDir, outFileName)
	return dc.SavePNG(outPath)
}

func generateCardBack(s *models.Song, qrImg image.Image, outputDir string) error {
	stdDir := filepath.Join(outputDir, outDirStdBackName)
	miniDir := filepath.Join(outputDir, outDirMiniBackName)

	if err := os.MkdirAll(stdDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(miniDir, 0755); err != nil {
		return err
	}

	if err := drawBack(qrImg, stdWidth, stdHeight, filepath.Join(stdDir, s.FileName()+".png")); err != nil {
		return err
	}
	return drawBack(qrImg, miniWidth, miniHeight, filepath.Join(miniDir, s.FileName()+".png"))
}

func drawBack(qrImg image.Image, widthIn, heightIn float64, outPath string) error {
	totalWidth := int((widthIn + 2*bleed) * dpi)
	totalHeight := int((heightIn + 2*bleed) * dpi)

	dst := image.NewRGBA(image.Rect(0, 0, totalWidth, totalHeight))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{cardBackgroundColor}, image.Point{}, draw.Src)

	safeW := (widthIn - 2*margin) * dpi
	safeH := (heightIn - 2*margin) * dpi
	qrSize := int(safeW)
	if safeH < safeW {
		qrSize = int(safeH)
	}

	cX := totalWidth / 2
	cY := totalHeight / 2
	qrX := cX - qrSize/2
	qrY := cY - qrSize/2
	qrRect := image.Rect(qrX, qrY, qrX+qrSize, qrY+qrSize)

	xdraw.CatmullRom.Scale(dst, qrRect, qrImg, qrImg.Bounds(), draw.Over, nil)

	dc := gg.NewContextForRGBA(dst)
	fontSize := float64(totalWidth) * 0.12
	if err := dc.LoadFontFace(fontPathBold, fontSize); err != nil {
		return err
	}
	dc.SetColor(color.White)
	text := "Temporalize"

	topTextY := float64(qrY) / 2.0
	dc.DrawStringAnchored(text, float64(cX), topTextY, 0.5, 0.5)

	bottomTextY := float64(qrY+qrSize+totalHeight) / 2.0
	dc.Push()
	dc.RotateAbout(gg.Radians(180), float64(cX), bottomTextY)
	dc.DrawStringAnchored(text, float64(cX), bottomTextY, 0.5, 0.5)
	dc.Pop()

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	return png.Encode(outFile, dst)
}

// Helpers

func loadFont(path string) (*truetype.Font, error) {
	fontBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return truetype.Parse(fontBytes)
}

func resizeImage(img image.Image, w, h int) image.Image {
	dc := gg.NewContext(w, h)
	sx := float64(w) / float64(img.Bounds().Dx())
	sy := float64(h) / float64(img.Bounds().Dy())
	dc.Scale(sx, sy)
	dc.DrawImage(img, 0, 0)
	return dc.Image()
}

func loadAndProcessIcon(path string, h int, tint color.Color) (image.Image, error) {
	img, err := gg.LoadImage(path)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	ratio := float64(bounds.Dx()) / float64(bounds.Dy())
	w := int(float64(h) * ratio)
	resized := resizeImage(img, w, h)
	return tintIcon(resized, tint), nil
}

func tintIcon(img image.Image, tint color.Color) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	tr, tg, tb, ta := tint.RGBA()

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.At(bounds.Min.X+x, bounds.Min.Y+y)
			_, _, _, a := c.RGBA()
			r, g, b, _ := c.RGBA()
			lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
			lumNorm := lum / 65535.0
			maskAlpha := 1.0 - lumNorm
			if maskAlpha < 0 {
				maskAlpha = 0
			}
			originalAlphaNorm := float64(a) / 65535.0
			finalAlphaNorm := maskAlpha * originalAlphaNorm
			newA := uint32(finalAlphaNorm * float64(ta))
			newR := uint32(float64(tr) * finalAlphaNorm)
			newG := uint32(float64(tg) * finalAlphaNorm)
			newB := uint32(float64(tb) * finalAlphaNorm)
			dst.Set(x, y, color.RGBA64{
				R: uint16(newR),
				G: uint16(newG),
				B: uint16(newB),
				A: uint16(newA),
			})
		}
	}
	return dst
}
