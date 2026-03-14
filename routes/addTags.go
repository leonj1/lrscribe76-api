package routes

import (
	"encoding/json"
	"github.com/husobee/vestigo"
	"net/http"
	"notes/models"
	"strconv"
)

func AddTags(w http.ResponseWriter, r *http.Request) {
	var tag models.Tag
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

	if r.Body == nil {
		http.Error(w, "Please send a request body", http.StatusBadRequest)
		return
	}
	err = json.NewDecoder(r.Body).Decode(&tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tag.NoteId = noteId

	tags, err := tag.FindByKeyAndValueAndNoteId(tag.Key, tag.Value, tag.NoteId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(*tags) > 0 {
		http.Error(w, "Tag already exists for this node", http.StatusBadRequest)
		return
	}

	saved, err := tag.Save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
