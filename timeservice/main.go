package main

import (
	"encoding/json"
	"net/http"
	"time"
)

type TimeResponse struct {
	Time string `json:"time"`
}

func main() {
	http.HandleFunc("GET /time", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TimeResponse{Time: time.Now().UTC().Format(time.RFC3339)})
	})
	http.ListenAndServe(":8080", nil)
}
