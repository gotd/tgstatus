package api

import (
	"context"
	"net"

	"github.com/gotd/tgstatus/internal/oas"
)

type Handler struct{}

func (h *Handler) Status(ctx context.Context) (oas.Status, error) {
	return oas.Status{
		Locations: []oas.Location{
			{
				ID: 1,
				Services: []oas.Service{
					{
						Status: oas.ServiceStatusOnline,
						IPV4:   oas.NewOptIP(net.ParseIP("149.154.175.52")),
					},
				},
			},
		},
	}, nil
}

var _ oas.Handler = (*Handler)(nil)
