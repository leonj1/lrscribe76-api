package routes

import (
	"encoding/json"
	"net/http"
	"notes/clients"
	"strings"
)

var getAuthUser = clients.GetAuthUser

func AuthUser(w http.ResponseWriter, r *http.Request) {
	token := authBearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeUnauthorized(w)
		return
	}

	user, err := getAuthUser(token)
	if err != nil {
		writeUnauthorized(w)
		return
	}

	js, err := json.Marshal(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.WriteHeader(http.StatusOK)
	w.Write(js)
}

func authBearerToken(header string) string {
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}
