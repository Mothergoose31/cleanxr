package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/mothergoose31/clean"
)

func saveImageAsPNG(img clean.Image, filename string) error {
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	for _, row := range img {
		for _, val := range row {
			if val < minVal {
				minVal = val
			}
			if val > maxVal {
				maxVal = val
			}
		}
	}
	imgWidth := len(img)
	imgHeight := len(img[0])
	pngImg := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	scale := 1.0
	if maxVal > minVal {
		scale = 1.0 / (maxVal - minVal)
	}

	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			if x < len(img) && y < len(img[x]) {
				normalizedValue := (img[x][y] - minVal) * scale
				pngImg.Set(x, y, clean.Viridis.ColorAt(normalizedValue))
			}
		}
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, pngImg)
}

func upsampleImage(img clean.Image, targetWidth, targetHeight int) clean.Image {
	sourceWidth := len(img)
	sourceHeight := len(img[0])

	result := make(clean.Image, targetWidth)
	for i := range result {
		result[i] = make([]float64, targetHeight)
	}

	xRatio := float64(sourceWidth-1) / float64(targetWidth-1)
	yRatio := float64(sourceHeight-1) / float64(targetHeight-1)

	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			srcX := float64(x) * xRatio
			srcY := float64(y) * yRatio

			x1, y1 := int(math.Floor(srcX)), int(math.Floor(srcY))
			x2, y2 := int(math.Ceil(srcX)), int(math.Ceil(srcY))

			if x2 >= sourceWidth {
				x2 = sourceWidth - 1
			}
			if y2 >= sourceHeight {
				y2 = sourceHeight - 1
			}

			weightX := srcX - float64(x1)
			weightY := srcY - float64(y1)

			topLeft := img[x1][y1]
			topRight := img[x2][y1]
			bottomLeft := img[x1][y2]
			bottomRight := img[x2][y2]

			top := topLeft*(1-weightX) + topRight*weightX
			bottom := bottomLeft*(1-weightX) + bottomRight*weightX

			result[x][y] = top*(1-weightY) + bottom*weightY
		}
	}

	return result
}

func main() {
	inputFile := flag.String("input", "", "Input ACB file")
	outputFile := flag.String("output", "cleaned_image.png", "Output image file")
	numScales := flag.Int("scales", 5, "Number of scales for Multi-scale CLEAN")
	imageSize := flag.Int("size", 256, "Size of the output image")
	highRes := flag.Bool("2k", false, "Generate 2K resolution image (2048x2048)")
	flag.Parse()
	if *inputFile == "" {
		fmt.Println("Please specify an input file with -input")
		os.Exit(1)
	}

	outputDir := filepath.Dir(*outputFile)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}
	fmt.Printf("Applying Multi-scale CLEAN to %s with %d scales...\n", *inputFile, *numScales)
	cleanedImage, err := clean.CleanACB(*inputFile, *numScales, *imageSize)
	if err != nil {
		log.Fatalf("Failed to clean ACB data: %v", err)
	}
	if *highRes {
		fmt.Println("Upsampling to 2K resolution...")
		cleanedImage = upsampleImage(cleanedImage, 2048, 2048)
	}
	fmt.Printf("Saving cleaned image to %s...\n", *outputFile)
	if err := saveImageAsPNG(cleanedImage, *outputFile); err != nil {
		log.Fatalf("Failed to save image: %v", err)
	}

	fmt.Println("Done!")
}
