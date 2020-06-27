package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gloworm-vision/gloworm-app/pipeline"
)

func (s *Server) pipelineSettings(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		if err := json.NewEncoder(res).Encode(s.Pipeline.Config()); err != nil {
			fmt.Println(err)
		}
		return
	}

	var config pipeline.Config
	if err := json.NewDecoder(req.Body).Decode(&config); err != nil {
		http.Error(res, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	s.Pipeline.SetConfig(config)
}
