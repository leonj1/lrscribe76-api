package routes

import (
	"encoding/json"
	"net/http"
	"notes/models"
)

func Search(w http.ResponseWriter, r *http.Request) {
	var n models.Note
	if r.Body == nil {
		http.Error(w, "Please send a request body", 400)
		return
	}
	err := json.NewDecoder(r.Body).Decode(&n)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	js, err := json.Marshal(n)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.Write(js)
}
