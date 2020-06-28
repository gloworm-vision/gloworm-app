package pipeline

import (
	"image"
	"image/color"
	"sort"

	"gocv.io/x/gocv"
)

type HSV struct {
	H float64 `json:"h"`
	S float64 `json:"s"`
	V float64 `json:"v"`
}

func (h HSV) scalar() gocv.Scalar {
	return gocv.Scalar{Val1: h.H, Val2: h.S, Val3: h.V}
}

type Config struct {
	MinThresh  HSV     `json:"minThresh"`
	MaxThresh  HSV     `json:"maxThresh"`
	MinContour float64 `json:"minContour"`
	MaxContour float64 `json:"maxContour"`
}

type Pipeline struct {
	Config Config
}

func New(config Config) Pipeline {
	return Pipeline{
		Config: config,
	}
}

type SortableContours [][]image.Point

func (s SortableContours) Len() int      { return len(s) }
func (s SortableContours) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s SortableContours) Less(i, j int) bool {
	if gocv.ContourArea(s[i]) < gocv.ContourArea(s[j]) {
		return true
	}

	return false
}

func calculateCentroid(img gocv.Mat, contour []image.Point) image.Point {
	mat := gocv.NewMatWithSize(img.Rows(), img.Cols(), gocv.MatTypeCV8U)
	gocv.FillPoly(&mat, [][]image.Point{contour}, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	moments := gocv.Moments(mat, false)

	x := int(moments["m10"] / moments["m00"])
	y := int(moments["m01"] / moments["m00"])

	return image.Point{X: x, Y: y}
}

func (p Pipeline) ProcessFrame(frame gocv.Mat, outFrame *gocv.Mat) (image.Point, bool) {
	frameHSV := gocv.NewMat()
	defer frameHSV.Close()
	gocv.CvtColor(frame, &frameHSV, gocv.ColorBGRToHSV)

	frameThresh := gocv.NewMat()
	defer frameThresh.Close()
	gocv.InRangeWithScalar(frameHSV, p.Config.MinThresh.scalar(), p.Config.MaxThresh.scalar(), &frameThresh)

	filteredContours := make([][]image.Point, 0)
	imageArea := float64(frameThresh.Rows() * frameThresh.Cols())

	for _, contour := range gocv.FindContours(frameThresh, gocv.RetrievalList, gocv.ChainApproxSimple) {
		area := gocv.ContourArea(contour)
		if area < p.Config.MinContour*imageArea || area > p.Config.MaxContour*imageArea {
			continue
		}

		rect := gocv.MinAreaRect(contour)
		gocv.Rectangle(outFrame, image.Rectangle{Min: rect.BoundingRect.Min, Max: rect.BoundingRect.Max}, color.RGBA{255, 255, 255, 255}, 2)

		filteredContours = append(filteredContours, contour)
	}

	sort.Sort(SortableContours(filteredContours))

	if len(filteredContours) > 0 {
		return calculateCentroid(frameThresh, filteredContours[0]), true
	}

	return image.Point{}, false
}
