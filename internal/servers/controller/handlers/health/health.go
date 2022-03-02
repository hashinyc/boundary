package health

import (
	"context"
	"net/http"

	pbs "github.com/hashicorp/boundary/internal/gen/controller/ops/services"
	"github.com/hashicorp/boundary/internal/servers/controller/handlers"
	"github.com/hashicorp/boundary/sdk/pbs/controller/ops/resources/health"
)

const (
	StatusHealthy  = "healthy"
	StatusShutdown = "shutdown"
)

type Service struct {
	pbs.UnimplementedHealthServiceServer

	replyWithServiceUnavailable bool
}

func NewService() (*Service, func()) {
	s := Service{}
	return &s, s.startServiceUnavailableReplies
}

func (s *Service) GetHealth(ctx context.Context, req *pbs.GetHealthRequest) (*pbs.GetHealthResponse, error) {
	if s.replyWithServiceUnavailable {
		err := handlers.SetStatusCode(ctx, http.StatusServiceUnavailable)
		if err != nil {
			return nil, err
		}
		return &pbs.GetHealthResponse{Health: &health.Health{Status: StatusShutdown}}, nil
	}

	return &pbs.GetHealthResponse{Health: &health.Health{Status: StatusHealthy}}, nil
}

// startServiceUnavailableReplies gets returned to the caller of NewService.
// When invoked, we start responding to any health queries with a 503.
func (s *Service) startServiceUnavailableReplies() {
	s.replyWithServiceUnavailable = true
}
