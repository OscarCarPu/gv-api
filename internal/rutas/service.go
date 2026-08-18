package rutas

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context) ([]ConcelloMark, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int32) (ConcelloMark, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, req CreateMarkRequest) (ConcelloMark, error) {
	return s.repo.Create(ctx, req)
}

func (s *Service) Update(ctx context.Context, req UpdateMarkRequest) (ConcelloMark, error) {
	return s.repo.Update(ctx, req)
}

func (s *Service) Delete(ctx context.Context, id int32) error {
	return s.repo.Delete(ctx, id)
}
