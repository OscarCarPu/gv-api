package varieties

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context, id int32) (Variety, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Variety, error) {
	return s.repo.List(ctx)
}

func (s *Service) Create(ctx context.Context, req CreateVarietyRequest) (Variety, error) {
	return s.repo.Create(ctx, req)
}

func (s *Service) Update(ctx context.Context, req UpdateVarietyRequest) (Variety, error) {
	return s.repo.Update(ctx, req)
}

func (s *Service) Delete(ctx context.Context, id int32) error {
	return s.repo.Delete(ctx, id)
}
