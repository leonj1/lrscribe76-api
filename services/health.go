package services

import "notes/clients"

type HealthService struct{}

func (h *HealthService) Check() string {
	return clients.HealthCheck()
}
