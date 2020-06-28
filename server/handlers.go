package server

import (
	"encoding/json"
	"net/http"

	"github.com/gloworm-vision/gloworm-app/hardware"
	"github.com/gloworm-vision/gloworm-app/pipeline"
	"github.com/julienschmidt/httprouter"
)

func (s *Server) getDefaultPipeline(res http.ResponseWriter, req *http.Request) {
	name, err := s.Store.DefaultPipelineConfig()
	if err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, name, http.StatusOK)
}

func (s *Server) putDefaultPipeline(res http.ResponseWriter, req *http.Request) {
	var name string
	if err := json.NewDecoder(req.Body).Decode(&name); err != nil {
		respond(res, err, http.StatusUnprocessableEntity)
		return
	}

	if err := s.Store.PutDefaultPipelineConfig(name); err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, nil, http.StatusNoContent)
}

func (s *Server) pipelines(res http.ResponseWriter, req *http.Request) {
	pipelines, err := s.Store.ListPipelineConfigs()
	if err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, pipelines, http.StatusOK)
}

func (s *Server) getPipeline(res http.ResponseWriter, req *http.Request) {
	params := httprouter.ParamsFromContext(req.Context())
	name := params.ByName("name")

	config, err := s.Store.PipelineConfig(name)
	if err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, config, http.StatusOK)
}

func (s *Server) putPipeline(res http.ResponseWriter, req *http.Request) {
	params := httprouter.ParamsFromContext(req.Context())
	name := params.ByName("name")

	var config pipeline.Config
	if err := json.NewDecoder(req.Body).Decode(&config); err != nil {
		respond(res, err, http.StatusUnprocessableEntity)
		return
	}

	err := s.Store.PutPipelineConfig(name, config)
	if err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, nil, http.StatusNoContent)
}

func (s *Server) getHardware(res http.ResponseWriter, req *http.Request) {
	config, err := s.Store.HardwareConfig()
	if err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, config, http.StatusOK)
}

func (s *Server) putHardware(res http.ResponseWriter, req *http.Request) {
	var hardware hardware.Config
	if err := json.NewDecoder(req.Body).Decode(&hardware); err != nil {
		respond(res, err, http.StatusUnprocessableEntity)
		return
	}

	if err := s.Store.PutHardwareConfig(hardware); err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, nil, http.StatusNoContent)
}

func (s *Server) updatePipeline(res http.ResponseWriter, req *http.Request) {
	name := req.URL.Query().Get("name")

	config, err := s.Store.PipelineConfig(name)
	if err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	s.pipelineManager.SetConfig(config)

	respond(res, nil, http.StatusOK)
}

func (s *Server) updateHardware(res http.ResponseWriter, req *http.Request) {
	config, err := s.Store.HardwareConfig()
	if err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	if err := s.hardwareManager.Update(config); err != nil {
		respond(res, err, http.StatusInternalServerError)
		return
	}

	respond(res, nil, http.StatusOK)
}
