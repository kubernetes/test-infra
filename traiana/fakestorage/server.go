package fakestorage

import (
	"context"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"k8s.io/test-infra/traiana"
	"k8s.io/test-infra/traiana/storage"
)

type Server struct {
	gcs *fakestorage.Server
}

func (s *Server) Stop() {
	if !traiana.Aws {
		s.gcs.Stop()
	}

	panic("not implemented")
}
func (s *Server) Client() *storage.Client {
	c, _ := storage.NewClient(context.Background())
	return c
}

func NewServer(o []Object) *Server {
	if !traiana.Aws {
		return &Server { gcs: fakestorage.NewServer(o)}
	}

	panic("not implemented")
}

