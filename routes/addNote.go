package routes

import (
	"encoding/json"
	"net/http"
	"notes/models"
)

const ContentType = "Content-Type"
const JSON = "application/json"

func AddNote(w http.ResponseWriter, r *http.Request) {
	var note models.Note
	if r.Body == nil {
		http.Error(w, "Please send a request body", http.StatusBadRequest)
		return
	}
	err := json.NewDecoder(r.Body).Decode(&note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	saved, err := note.Save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	js, err := json.Marshal(&saved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.Write(js)
}
