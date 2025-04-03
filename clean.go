package clean

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type Point struct {
	x, y int
}

type Image [][]float64
type PFS []Image

const (
	gainFactor = 0.1
)

type ACBData struct {
	TimeRange     string
	ObsCode       string
	Channels      string
	Source        string
	Bandwidth     string
	Frequencies   []float64
	Polarizations []string
	Amplitudes    []float64
}

type workerPool struct {
	workers int
	wg      sync.WaitGroup
}

func newWorkerPool() *workerPool {
	return &workerPool{
		workers: (runtime.NumCPU() * 3) / 4,
	}
}

func (p *workerPool) divide(total int) [][2]int {
	chunks := make([][2]int, p.workers)
	chunkSize := total / p.workers
	remainder := total % p.workers

	start := 0
	for i := 0; i < p.workers; i++ {
		size := chunkSize
		if i < remainder {
			size++
		}
		chunks[i] = [2]int{start, start + size}
		start += size
	}
	return chunks
}

func ParseACB(filename string) (*ACBData, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open ACB file: %v", err)
	}
	defer file.Close()

	data := &ACBData{
		Frequencies:   []float64{},
		Polarizations: []string{},
		Amplitudes:    []float64{},
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if lineNum == 1 {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "timerange:" && i+4 < len(parts) {
					data.TimeRange = strings.Join(parts[i+1:i+5], " ")
				} else if part == "obscode:" && i+1 < len(parts) {
					data.ObsCode = parts[i+1]
				} else if part == "chans:" && i+3 < len(parts) {
					data.Channels = strings.Join(parts[i+1:i+4], " ")
				}
			}
		} else if strings.HasPrefix(line, "source:") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "source:" && i+1 < len(parts) {
					data.Source = parts[i+1]
				} else if part == "bandw:" && i+2 < len(parts) {
					data.Bandwidth = parts[i+1] + " " + parts[i+2]
				}
			}
		} else if strings.HasPrefix(line, "bandfreq:") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "bandfreq:" && i+2 < len(parts) {
					freq, err := strconv.ParseFloat(parts[i+1], 64)
					if err == nil {
						data.Frequencies = append(data.Frequencies, freq)
					}
				} else if part == "polar:" && i+1 < len(parts) {
					data.Polarizations = append(data.Polarizations, parts[i+1])
				}
			}
		} else if strings.HasPrefix(line, " 1 LM") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				amp, err := strconv.ParseFloat(parts[3], 64)
				if err == nil {
					data.Amplitudes = append(data.Amplitudes, amp)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning ACB file: %v", err)
	}

	return data, nil
}

func CleanACB(filename string, numScales int, imageSize int) (Image, error) {
	data, err := ParseACB(filename)
	if err != nil {
		return nil, err
	}

	dirtyMaps := createDirtyMapsFromACB(data, numScales, imageSize)
	psfs := createPSFsFromACB(numScales, imageSize)
	basisFuncs := createBasisFunctionsFromACB(numScales, imageSize)

	scaleBias := make([]float64, numScales)
	for i := range scaleBias {
		scaleBias[i] = 1.0 / math.Sqrt(float64(i+1))
	}
	cleanedImage := MultiScaleClean(dirtyMaps, psfs, basisFuncs, scaleBias)

	return cleanedImage, nil
}

func createDirtyMapsFromACB(data *ACBData, numScales int, imageSize int) PFS {
	fmt.Println("Creating dirty maps from ACB data...")
	dirtyMaps := make(PFS, numScales)
	for s := 0; s < numScales; s++ {
		dirtyMaps[s] = make(Image, imageSize)
		for i := range dirtyMaps[s] {
			dirtyMaps[s][i] = make([]float64, imageSize)
		}
	}

	center := imageSize / 2
	uniqueFreqs := getUniqueFrequencies(data.Frequencies)
	fmt.Printf("Found %d unique frequencies\n", len(uniqueFreqs))
	fmt.Printf("Using %d amplitude values\n", len(data.Amplitudes))

	scaleSigmas := make([]float64, numScales)
	for s := 0; s < numScales; s++ {
		scaleSigmas[s] = 1.0 + float64(s)*2.0
	}

	gaussianLookup := make([][][]float64, numScales)
	var wg sync.WaitGroup
	wg.Add(numScales)

	for s := 0; s < numScales; s++ {
		go func(scale int) {
			defer wg.Done()
			gaussianLookup[scale] = make([][]float64, imageSize)
			for i := range gaussianLookup[scale] {
				gaussianLookup[scale][i] = make([]float64, imageSize)
				for j := range gaussianLookup[scale][i] {
					dx := float64(i - center)
					dy := float64(j - center)
					distance := math.Sqrt(dx*dx + dy*dy)
					gaussianLookup[scale][i][j] = math.Exp(-(distance * distance) / (2 * scaleSigmas[scale] * scaleSigmas[scale]))
				}
			}
		}(s)
	}
	wg.Wait()
	pool := newWorkerPool()
	chunks := pool.divide(len(data.Amplitudes))
	pool.wg.Add(len(chunks))
	var mutex sync.Mutex
	for _, chunk := range chunks {
		go func(start, end int) {
			defer pool.wg.Done()

			for i := start; i < end && i < len(data.Amplitudes); i++ {
				amp := data.Amplitudes[i]
				if i >= len(uniqueFreqs) {
					continue
				}

				scaleIndex := int(float64(i) / float64(len(uniqueFreqs)) * float64(numScales))
				if scaleIndex >= numScales {
					scaleIndex = numScales - 1
				}

				localUpdates := make([][]float64, imageSize)
				for x := range localUpdates {
					localUpdates[x] = make([]float64, imageSize)
					for y := 0; y < imageSize; y++ {
						localUpdates[x][y] = amp * gaussianLookup[scaleIndex][x][y]
					}
				}

				mutex.Lock()
				for x := 0; x < imageSize; x++ {
					for y := 0; y < imageSize; y++ {
						dirtyMaps[scaleIndex][x][y] += localUpdates[x][y]
					}
				}
				mutex.Unlock()
			}
		}(chunk[0], chunk[1])
	}
	pool.wg.Wait()

	return dirtyMaps
}

func createPSFsFromACB(numScales int, imageSize int) PFS {
	fmt.Println("Creating PSFs...")
	psfs := make(PFS, numScales)
	for s := 0; s < numScales; s++ {
		psfs[s] = make(Image, imageSize)
		for i := range psfs[s] {
			psfs[s][i] = make([]float64, imageSize)
		}
	}

	center := imageSize / 2
	fmt.Println("Generating PSF patterns...")

	for s := 0; s < numScales; s++ {
		fmt.Printf("  Scale %d/%d...\n", s+1, numScales)
		sigma := 1.0 + float64(s)*0.5

		for x := 0; x < imageSize; x++ {
			for y := 0; y < imageSize; y++ {
				dx := float64(x - center)
				dy := float64(y - center)
				distance := math.Sqrt(dx*dx + dy*dy)
				psfs[s][x][y] = math.Exp(-(distance * distance) / (2 * sigma * sigma))
			}
		}

		fmt.Println("  Normalizing PSF...")
		total := 0.0
		for x := 0; x < imageSize; x++ {
			for y := 0; y < imageSize; y++ {
				total += psfs[s][x][y]
			}
		}
		if total > 0 {
			for x := 0; x < imageSize; x++ {
				for y := 0; y < imageSize; y++ {
					psfs[s][x][y] /= total
				}
			}
		}
	}

	return psfs
}

func createBasisFunctionsFromACB(numScales int, imageSize int) PFS {
	basisFuncs := make(PFS, numScales)
	for s := 0; s < numScales; s++ {
		basisFuncs[s] = make(Image, imageSize)
		for i := range basisFuncs[s] {
			basisFuncs[s][i] = make([]float64, imageSize)
		}
	}
	center := imageSize / 2
	for s := 0; s < numScales; s++ {
		sigma := 1.0 + float64(s)*2.0
		for x := 0; x < imageSize; x++ {
			for y := 0; y < imageSize; y++ {
				dx := float64(x - center)
				dy := float64(y - center)
				distance := math.Sqrt(dx*dx + dy*dy)
				basisFuncs[s][x][y] = math.Exp(-(distance * distance) / (2 * sigma * sigma))
			}
		}
	}

	return basisFuncs
}

func getUniqueFrequencies(frequencies []float64) []float64 {
	seen := make(map[float64]bool)
	unique := []float64{}
	for _, freq := range frequencies {
		if !seen[freq] {
			seen[freq] = true
			unique = append(unique, freq)
		}
	}

	return unique
}

func MultiScaleClean(unclean PFS, psfs PFS, basisFuncs PFS, scaleBias []float64) Image {
	fmt.Println("Starting Multi-scale CLEAN algorithm...")
	numScales := len(unclean)
	cleanComponents := make(Image, len(unclean[0]))
	for i := range cleanComponents {
		cleanComponents[i] = make([]float64, len(unclean[0][i]))
	}
	currentDirtyMaps := make([]Image, numScales)
	for i := range currentDirtyMaps {
		currentDirtyMaps[i] = make(Image, len(unclean[i]))
		for j := range currentDirtyMaps[i] {
			currentDirtyMaps[i][j] = make([]float64, len(unclean[i][j]))
			copy(currentDirtyMaps[i][j], unclean[i][j])
		}
	}

	maxIterations := 50
	iterCount := 0
	fmt.Println("Beginning iterations...")

	for iterCount < maxIterations {
		fmt.Printf("Iteration %d/%d...\n", iterCount+1, maxIterations)
		rescaledDirtyMaps := rescaleDirtyMaps(currentDirtyMaps, scaleBias)
		maxScale := identifyMaxScale(rescaledDirtyMaps)
		fmt.Printf("  Selected scale: %d\n", maxScale)
		maxPos, maxIntensity := identifyMaxPosition(currentDirtyMaps[maxScale])
		fmt.Printf("  Max position: (%d, %d), intensity: %f\n", maxPos.x, maxPos.y, maxIntensity)
		if maxIntensity < 1e-5 {
			fmt.Println("  Maximum intensity too low, stopping.")
			break
		}
		fmt.Println("  Updating clean components...")
		updateCleanComponents(cleanComponents, basisFuncs[maxScale], maxPos, maxIntensity, psfs[maxScale])
		fmt.Println("  Updating dirty maps...")
		updateDirtyMaps(currentDirtyMaps, basisFuncs[maxScale], maxPos, maxIntensity, psfs)
		if stoppingCondition(currentDirtyMaps) {
			fmt.Println("  Stopping condition met, ending iterations.")
			break
		}
		iterCount++
	}
	fmt.Printf("Multi-scale CLEAN completed in %d iterations\n", iterCount)
	fmt.Println("Adding residuals...")
	cleanedImage := addResiduals(cleanComponents, currentDirtyMaps)
	return cleanedImage
}

func rescaleDirtyMaps(dirtyMaps []Image, scaleBias []float64) []Image {
	rescaled := make([]Image, len(dirtyMaps))
	for i := range rescaled {
		rescaled[i] = make(Image, len(dirtyMaps[i]))
		for j := range rescaled[i] {
			rescaled[i][j] = make([]float64, len(dirtyMaps[i][j]))
			for k := range rescaled[i][j] {
				rescaled[i][j][k] = scaleBias[i] * dirtyMaps[i][j][k]
			}
		}
	}
	return rescaled
}

func identifyMaxScale(rescaledDirtyMaps []Image) int {
	maxScale := 0
	maxIntensity := math.Inf(-1)

	for i, img := range rescaledDirtyMaps {
		for _, row := range img {
			for _, val := range row {
				if val > maxIntensity {
					maxIntensity = val
					maxScale = i
				}
			}
		}
	}

	return maxScale
}

func identifyMaxPosition(img Image) (Point, float64) {
	maxPos := Point{}
	maxIntensity := math.Inf(-1)

	for i, row := range img {
		for j, val := range row {
			if val > maxIntensity {
				maxIntensity = val
				maxPos = Point{x: i, y: j}
			}
		}
	}

	return maxPos, maxIntensity
}

func updateCleanComponents(cleanComponents Image, basisFunction Image, maxPos Point, maxIntensity float64, psf Image) {
	normFactor := maxIntensity / maxValue(convolve(basisFunction, psf))
	for i := range basisFunction {
		for j := range basisFunction[i] {
			x := maxPos.x + i - len(basisFunction)/2
			y := maxPos.y + j - len(basisFunction[i])/2
			if x >= 0 && x < len(cleanComponents) && y >= 0 && y < len(cleanComponents[x]) {
				cleanComponents[x][y] += gainFactor * normFactor * basisFunction[i][j]
			}
		}
	}
}

// TODO: This is a bottleneck need to find a better way to do this
func updateDirtyMaps(dirtyMaps []Image, basisFunction Image, maxPos Point, maxIntensity float64, psfs []Image) {
	pool := newWorkerPool()
	var wg sync.WaitGroup
	crossConvs := make([]Image, len(dirtyMaps))
	wg.Add(len(dirtyMaps))
	for i := range dirtyMaps {
		go func(idx int) {
			defer wg.Done()
			crossConvs[idx] = convolve(basisFunction, psfs[idx])
		}(i)
	}
	wg.Wait()
	chunks := pool.divide(len(dirtyMaps))
	pool.wg.Add(len(chunks))
	for _, chunk := range chunks {
		go func(start, end int) {
			defer pool.wg.Done()

			for i := start; i < end; i++ {
				crossConv := crossConvs[i]
				normFactor := gainFactor * maxIntensity / maxValue(crossConv)

				blockSize := 32
				for j := 0; j < len(crossConv); j += blockSize {
					for k := 0; k < len(crossConv[0]); k += blockSize {
						endJ := min(j+blockSize, len(crossConv))
						endK := min(k+blockSize, len(crossConv[0]))

						for jj := j; jj < endJ; jj++ {
							for kk := k; kk < endK; kk++ {
								x := maxPos.x + jj - len(crossConv)/2
								y := maxPos.y + kk - len(crossConv[0])/2
								if x >= 0 && x < len(dirtyMaps[i]) && y >= 0 && y < len(dirtyMaps[i][x]) {
									dirtyMaps[i][x][y] -= normFactor * crossConv[jj][kk]
								}
							}
						}
					}
				}
			}
		}(chunk[0], chunk[1])
	}
	pool.wg.Wait()
}
func stoppingCondition(dirtyMaps []Image) bool {
	threshold := 1e-5
	for _, img := range dirtyMaps {
		for _, row := range img {
			for _, val := range row {
				if math.Abs(val) > threshold {
					return false
				}
			}
		}
	}
	return true
}

func addResiduals(cleanComponents Image, dirtyMaps []Image) Image {
	residualMap := make(Image, len(dirtyMaps[0]))
	for i := range residualMap {
		residualMap[i] = make([]float64, len(dirtyMaps[0][i]))
	}

	for _, img := range dirtyMaps {
		for i := range img {
			for j := range img[i] {
				residualMap[i][j] += img[i][j]
			}
		}
	}
	cleanedImage := make(Image, len(cleanComponents))
	for i := range cleanedImage {
		cleanedImage[i] = make([]float64, len(cleanComponents[i]))
		for j := range cleanedImage[i] {
			cleanedImage[i][j] = cleanComponents[i][j] + residualMap[i][j]
		}
	}

	return cleanedImage
}

func convolve(img1, img2 Image) Image {
	h1, w1 := len(img1), len(img1[0])
	h2, w2 := len(img2), len(img2[0])
	h := h1 + h2 - 1
	w := w1 + w2 - 1

	result := make(Image, h)
	for i := range result {
		result[i] = make([]float64, w)
	}

	pool := newWorkerPool()
	chunks := pool.divide(h1)

	pool.wg.Add(len(chunks))
	for _, chunk := range chunks {
		go func(start, end int) {
			defer pool.wg.Done()
			for i := start; i < end; i++ {
				for j := 0; j < w1; j++ {
					for k := 0; k < h2; k++ {
						for l := 0; l < w2; l++ {
							result[i+k][j+l] += img1[i][j] * img2[k][l]
						}
					}
				}
			}
		}(chunk[0], chunk[1])
	}
	pool.wg.Wait()

	return result
}

func maxValue(img Image) float64 {
	maxVal := math.Inf(-1)
	for _, row := range img {
		for _, val := range row {
			if val > maxVal {
				maxVal = val
			}
		}
	}
	return maxVal
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
