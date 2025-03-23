package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/mothergoose31/clean"
)

func main() {
	// Parse command-line arguments
	inputFile := flag.String("input", "", "Input ACB file")
	outputFile := flag.String("output", "cleaned_image.png", "Output image file")
	numScales := flag.Int("scales", 5, "Number of scales for Multi-scale CLEAN")
	imageSize := flag.Int("size", 256, "Size of the output image")
	flag.Parse()

	if *inputFile == "" {
		fmt.Println("Please specify an input file with -input")
		os.Exit(1)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(*outputFile)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	// Apply Multi-scale CLEAN to the ACB data
	fmt.Printf("Applying Multi-scale CLEAN to %s with %d scales...\n", *inputFile, *numScales)
	cleanedImage, err := clean.CleanACB(*inputFile, *numScales, *imageSize)
	if err != nil {
		log.Fatalf("Failed to clean ACB data: %v", err)
	}

	// Save the cleaned image as PNG
	fmt.Printf("Saving cleaned image to %s...\n", *outputFile)
	if err := saveImageAsPNG(cleanedImage, *outputFile); err != nil {
		log.Fatalf("Failed to save image: %v", err)
	}

	fmt.Println("Done!")
}

// saveImageAsPNG converts our Image type to a PNG file
func saveImageAsPNG(img clean.Image, filename string) error {
	// Find min and max values for normalization
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

	// Create a new grayscale image
	imgWidth := len(img)
	imgHeight := len(img[0])
	pngImg := image.NewGray(image.Rect(0, 0, imgWidth, imgHeight))

	// Normalize values to 0-255 range
	scale := 255.0
	if maxVal > minVal {
		scale = 255.0 / (maxVal - minVal)
	}

	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			if x < len(img) && y < len(img[x]) {
				normalized := (img[x][y] - minVal) * scale
				if normalized < 0 {
					normalized = 0
				}
				if normalized > 255 {
					normalized = 255
				}
				pngImg.SetGray(x, y, color.Gray{uint8(normalized)})
			}
		}
	}

	// Save the image
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, pngImg)
}
