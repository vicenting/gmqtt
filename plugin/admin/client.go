package admin

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
)

type clientService struct {
	a *Admin
}

func (c *clientService) mustEmbedUnimplementedClientServiceServer() {
	return
}

// List lists clients information which the session is valid in the broker (both connected and disconnected).
func (c *clientService) List(ctx context.Context, req *ListClientRequest) (*ListClientResponse, error) {
	page, pageSize := getPage(req.Page, req.PageSize)
	clients, total, err := c.a.store.GetClients(page, pageSize)
	if err != nil {
		return &ListClientResponse{}, err
	}
	return &ListClientResponse{
		Clients:    clients,
		TotalCount: total,
	}, nil
}

// Get returns the client information for given request client id.
func (c *clientService) Get(ctx context.Context, req *GetClientRequest) (*GetClientResponse, error) {
	if req.ClientId == "" {
		return nil, InvalidArgument("client_id", "")
	}
	client := c.a.store.GetClientByID(req.ClientId)
	return &GetClientResponse{
		Client: client,
	}, nil
}

// Delete force disconnect.
func (c *clientService) Delete(ctx context.Context, req *DeleteClientRequest) (*empty.Empty, error) {
	if req.ClientId == "" {
		return nil, InvalidArgument("client_id", "")
	}
	if req.CleanSession {
		c.a.clientService.TerminateSession(req.ClientId)
	} else {
		c := c.a.clientService.GetClient(req.ClientId)
		if c != nil {
			c.Close()
		}
	}
	return &empty.Empty{}, nil
}
