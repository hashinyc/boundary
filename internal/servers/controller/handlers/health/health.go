package health

import (
	"context"
	"net/http"

	pbs "github.com/hashicorp/boundary/internal/gen/controller/api/services"
	"github.com/hashicorp/boundary/internal/servers/controller/handlers"
	"github.com/hashicorp/boundary/sdk/pbs/controller/api/resources/health"
)

const (
	StatusHealthy  = "healthy"
	StatusShutdown = "shutdown"
)

type Service struct {
	pbs.UnimplementedHealthServiceServer

	shuttingDownChan chan struct{}
	isShuttingDown   bool
}

func NewService() (*Service, chan struct{}) {
	s := Service{shuttingDownChan: make(chan struct{})}
	go s.waitForShutdownSignal()

	return &s, s.shuttingDownChan
}

func (s *Service) GetHealth(ctx context.Context, req *pbs.GetHealthRequest) (*pbs.GetHealthResponse, error) {
	if s.isShuttingDown {
		err := handlers.SetStatusCode(ctx, http.StatusServiceUnavailable)
		if err != nil {
			return nil, err
		}
		return &pbs.GetHealthResponse{Health: &health.Health{Status: StatusShutdown}}, nil
	}

	return &pbs.GetHealthResponse{Health: &health.Health{Status: StatusHealthy}}, nil
}

func (s *Service) waitForShutdownSignal() {
	<-s.shuttingDownChan
	s.isShuttingDown = true
}
