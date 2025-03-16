package clean

type Point struct {
	x, y int
}

type Image [][]float64
type PFS []Image

const (
	gainFactor = 0.1
)

func MultiScaleClean(unclean PFS, psfs PFS, basisFuncs PFS, scaleBias []float64) Image {
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

	for {

	}

	return cleanedImage
}
