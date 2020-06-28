package server

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

// respond encodes the data and ResponseError to JSON and responds with it and
// the http code. If the encoding fails, sets an InternalServerError.
func respond(w http.ResponseWriter, data interface{}, httpCode int) {
	var resp interface{}
	if v, ok := data.(error); ok {
		resp = errorResponse{Error: v.Error()}
	} else {
		resp = data
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)

	if resp != nil {
		_ = json.NewEncoder(w).Encode(resp)
	}
}
