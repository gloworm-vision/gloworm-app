package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gloworm-vision/gloworm-app/hardware"
	"github.com/gloworm-vision/gloworm-app/pipeline"
	"github.com/gloworm-vision/gloworm-app/store"
	"github.com/hybridgroup/mjpeg"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
)

type Server struct {
	Addr string

	Store   store.Store
	Capture *gocv.VideoCapture
	Logger  *logrus.Logger

	stream *mjpeg.Stream

	pipelineManager *pipelineManager
	hardwareManager *hardwareManager
}

func (s *Server) Run(ctx context.Context) error {
	s.stream = mjpeg.NewStream()

	if err := s.init(); err != nil {
		return fmt.Errorf("unable to initialize: %w", err)
	}

	mux := httprouter.New()

	mux.Handler(http.MethodGet, "/stream", s.stream)

	mux.HandlerFunc(http.MethodGet, "/pipeline", s.getDefaultPipeline)
	mux.HandlerFunc(http.MethodPut, "/pipeline", s.putDefaultPipeline)
	mux.HandlerFunc(http.MethodGet, "/pipelines", s.pipelines)
	mux.HandlerFunc(http.MethodGet, "/pipelines/:name", s.getPipeline)
	mux.HandlerFunc(http.MethodPut, "/pipelines/:name", s.putPipeline)

	mux.HandlerFunc(http.MethodGet, "/hardware", s.getHardware)
	mux.HandlerFunc(http.MethodPut, "/hardware", s.putHardware)

	mux.HandlerFunc(http.MethodPost, "/rpc/updatePipeline", s.updatePipeline)
	mux.HandlerFunc(http.MethodPost, "/rpc/updateHardware", s.updateHardware)

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

// init attempts to initialize the hardware manager and pipeline manager
// with configs from the store
func (s *Server) init() error {
	s.hardwareManager = &hardwareManager{mu: new(sync.RWMutex)}

	config, err := s.Store.HardwareConfig()
	if err == nil {
		hardware, err := hardware.New(config)
		if err == nil {
			s.hardwareManager.hardware = hardware
		} else {
			s.Logger.Warnf("unable to setup new hardware: %s", err)
		}
	} else {
		s.Logger.Warnf("no hardware config found: %s", err)
	}

	s.pipelineManager = &pipelineManager{mu: new(sync.RWMutex)}

	defaultConfig, err := s.Store.DefaultPipelineConfig()
	if err == nil {
		config, err := s.Store.PipelineConfig(defaultConfig)
		if err == nil {
			s.pipelineManager.pipeline = &pipeline.Pipeline{Config: config}
		} else {
			s.Logger.Warnf("unable to setup default pipeline config: %s", err)
		}
	} else {
		s.Logger.Warnf("no default pipeline config found: %s", err)
	}

	return nil
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

			pipeline := s.pipelineManager.Pipeline()
			if pipeline != nil {
				s.Logger.Debug("pipeline processing")
				point, ok := pipeline.ProcessFrame(frameBuffer, &frameBuffer)

				s.Logger.Infof("point: %v, ok: %v", point, ok)
			}

			buf, err := gocv.IMEncode(".jpg", frameBuffer)
			if err != nil {
				return fmt.Errorf("encode original frame buffer: %w", err)
			}

			s.stream.UpdateJPEG(buf)
		}
	}
}
