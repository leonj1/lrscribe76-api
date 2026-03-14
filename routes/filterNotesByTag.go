package routes

import (
	"encoding/json"
	"github.com/husobee/vestigo"
	"net/http"
	"notes/models"
)

func FilterNotesByTag(w http.ResponseWriter, r *http.Request) {
	var t models.Tag
	var n models.Note

	key := vestigo.Param(r, "key")
	if key == "" {
		http.Error(w, "Invalid key", http.StatusBadRequest)
		return
	}
	value := vestigo.Param(r, "value")
	if value == "" {
		http.Error(w, "Invalid value", http.StatusBadRequest)
		return
	}

	tags, err := t.FindByKeyAndValue(key, value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ids := make([]int64, 0)
	for _, tag := range tags {
		ids = append(ids, tag.NoteId)
	}

	notes, err := n.FindIn(&ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	js, err := json.Marshal(notes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.Write(js)
}
