package routes

import (
	"net/http"
	"notes/services"
)

func Health(w http.ResponseWriter, r *http.Request) {
	svc := &services.HealthService{}
	w.Header().Set(ContentType, JSON)
	w.Write([]byte(svc.Check()))
}
