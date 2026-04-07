package utils

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// OkJSON writes a 200 response with the given value as JSON.
func OkJSON(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusOK, v)
}

// CreatedJSON writes a 201 response.
func CreatedJSON(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusCreated, v)
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// ErrorJSON writes an error response.
func ErrorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// AcceptedJSON writes a 202 response.
func AcceptedJSON(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusAccepted, v)
}
