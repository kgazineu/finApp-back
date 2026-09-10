package user

type CreateInput struct {
	Name     string
	Email    string
	Password string
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}
