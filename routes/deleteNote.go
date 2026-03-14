package routes

import (
	"encoding/json"
	"github.com/husobee/vestigo"
	"net/http"
	"notes/models"
	"strconv"
)

func DeleteNote(w http.ResponseWriter, r *http.Request) {
	var note models.Note

	id := vestigo.Param(r, "id")
	if id == "" {
		http.Error(w, "Invalid note_id", http.StatusBadRequest)
		return
	}

	noteId, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		http.Error(w, "Invalid note_id", http.StatusBadRequest)
		return
	}

	err = note.DeleteNodeById(noteId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO Verify the returned json looks good to the client
	msg := []byte(`{"status":"deleted"}`)

	js, err := json.Marshal(&msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.Write(js)
}
