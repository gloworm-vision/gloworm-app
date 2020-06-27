package pipeline

import (
	"image/color"
	"sync"

	"gocv.io/x/gocv"
)

type HSV struct {
	H float64 `json:"h"`
	S float64 `json:"s"`
	V float64 `json:"v"`
}

func (h HSV) scalar() gocv.Scalar {
	return gocv.NewScalar(h.H, h.S, h.V, 0)
}

type Config struct {
	MinThresh      HSV     `json:"minThresh"`
	MaxThresh      HSV     `json:"maxThresh"`
	MinContourArea float64 `json:"minContourArea"`
	MaxContourArea float64 `json:"maxContourArea"`
}

type Pipeline struct {
	mu     *sync.Mutex
	config Config
}

func New(config Config) *Pipeline {
	return &Pipeline{
		config: config,
		mu:     new(sync.Mutex),
	}
}

func (p *Pipeline) SetConfig(config Config) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = config
}

func (p *Pipeline) Config() Config {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.config
}

func (p *Pipeline) ProcessFrame(frame gocv.Mat, outFrame *gocv.Mat) []gocv.Point2f {
	c := p.Config()

	frameHSV := gocv.NewMat()
	defer frameHSV.Close()
	gocv.CvtColor(frame, &frameHSV, gocv.ColorBGRToHSV)

	frameThresh := gocv.NewMat()
	defer frameThresh.Close()
	gocv.InRangeWithScalar(frameHSV, c.MinThresh.scalar(), c.MaxThresh.scalar(), &frameThresh)

	contours := gocv.FindContours(frameThresh, gocv.RetrievalList, gocv.ChainApproxSimple)

	for contourIndex, contour := range contours {
		area := gocv.ContourArea(contour)
		if area < c.MinContourArea || area > c.MaxContourArea {
			continue
		}

		gocv.DrawContours(outFrame, contours, contourIndex, color.RGBA{0xff, 0xff, 0xff, 0xff}, 2)
	}

	return []gocv.Point2f{{X: 1, Y: 2}}
}
