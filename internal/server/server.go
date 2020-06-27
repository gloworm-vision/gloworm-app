package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gloworm-vision/gloworm-app/hardware"
	"github.com/gloworm-vision/gloworm-app/pipeline"
	"github.com/hybridgroup/mjpeg"
	"github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
)

type Server struct {
	Addr string

	Hardware hardware.Hardware
	Capture  *gocv.VideoCapture
	Pipeline *pipeline.Pipeline
	Logger   *logrus.Logger

	originalStream *mjpeg.Stream
	pipelineStream *mjpeg.Stream
}

func (s *Server) Run(ctx context.Context) error {
	s.originalStream = mjpeg.NewStream()
	s.pipelineStream = mjpeg.NewStream()

	mux := http.NewServeMux()
	mux.Handle("/streams/original", s.originalStream)
	mux.Handle("/streams/pipeline", s.pipelineStream)
	mux.HandleFunc("/pipeline", s.pipelineSettings)

	httpServer := &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadTimeout:       time.Second * 15,
		ReadHeaderTimeout: time.Second * 15,
		IdleTimeout:       time.Second * 30,
		MaxHeaderBytes:    4096,
	}

	listenErrs := make(chan error)
	go func() {
		s.Logger.WithField("addr", s.Addr).Info("serving http")
		listenErrs <- httpServer.ListenAndServe()
	}()

	visionCtx, cancelVision := context.WithCancel(ctx)
	defer cancelVision()

	visionErrs := make(chan error)
	go func() {
		s.Logger.Info("starting vision loop")
		visionErrs <- s.runVision(visionCtx)
	}()

	select {
	case err := <-listenErrs:
		return err
	case err := <-visionErrs:
		httpServer.Shutdown(ctx)
		return err
	case <-ctx.Done():
		return httpServer.Shutdown(ctx)
	}
}

func (s *Server) runVision(ctx context.Context) error {
	frameBuffer := gocv.NewMat()
	defer frameBuffer.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if s.Capture.Read(&frameBuffer) == false {
				return errors.New("couldn't read from capture")
			}

			out := gocv.NewMatWithSize(frameBuffer.Rows(), frameBuffer.Cols(), frameBuffer.Type())
			_ = s.Pipeline.ProcessFrame(frameBuffer, &out)

			origBytes, err := gocv.IMEncode(".jpg", frameBuffer)
			if err != nil {
				return fmt.Errorf("encode original frame buffer: %w", err)
			}
			s.originalStream.UpdateJPEG(origBytes)

			outBytes, err := gocv.IMEncode(".jpg", out)
			if err != nil {
				return fmt.Errorf("encode pipeline output buffer: %w", err)
			}
			s.pipelineStream.UpdateJPEG(outBytes)
		}
	}
}