package transaction

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidUserID         = errors.New("ID de usuário inválido")
	ErrInvalidAmount         = errors.New("valor deve ser maior que zero")
	ErrInvalidNecessityLevel = errors.New("nível de necessidade deve ser 0 para receita ou entre 1 e 5 para despesa")
	ErrInvalidKind           = errors.New("tipo deve ser income ou expense")
	ErrInvalidDescription    = errors.New("descrição deve ter entre 1 e 200 caracteres")
	ErrInvalidCategory       = errors.New("categoria deve ter entre 1 e 80 caracteres")
	ErrInvalidPaymentMethod  = errors.New("forma de pagamento inválida para o tipo de transação")
	ErrInvalidInstallments   = errors.New("parcelas devem estar entre 1 e 60; múltiplas parcelas exigem cartão")
	ErrInvalidOccurredAt     = errors.New("data da transação obrigatória")
)

type Transaction struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// AmountMinor represents the amount in the currency's smallest unit, not a floating-point value.
	AmountMinor    int64
	NecessityLevel int // 0 for income; stored as NULL in PostgreSQL.
	Kind           string
	Description    string
	Category       string
	PaymentMethod  string
	Installments   int
	OccurredAt     time.Time
	CreatedAt      time.Time
}

func New(userID uuid.UUID, amountMinor int64, necessityLevel int) (Transaction, error) {
	return NewDetailed(CreateInput{UserID: userID, AmountMinor: amountMinor, NecessityLevel: necessityLevel})
}

// NewDetailed accepts the old three-field input as a legacy expense. Enriched
// inputs must specify kind and all required details explicitly.
func NewDetailed(input CreateInput) (Transaction, error) {
	if input.UserID == uuid.Nil {
		return Transaction{}, ErrInvalidUserID
	}
	if input.AmountMinor <= 0 {
		return Transaction{}, ErrInvalidAmount
	}
	legacy := !input.KindProvided && input.Kind == "" && input.Description == "" && input.Category == "" &&
		input.PaymentMethod == "" && input.Installments == 0 && input.OccurredAt.IsZero()
	if legacy {
		input.Kind = "expense"
		input.Description = "Lançamento"
		input.Category = "Outros"
		input.Installments = 1
		input.OccurredAt = time.Now().UTC()
	}
	if input.Kind != "income" && input.Kind != "expense" {
		return Transaction{}, ErrInvalidKind
	}
	if (input.Kind == "income" && input.NecessityLevel != 0) ||
		(input.Kind == "expense" && (input.NecessityLevel < 1 || input.NecessityLevel > 5)) {
		return Transaction{}, ErrInvalidNecessityLevel
	}
	input.Description = strings.TrimSpace(input.Description)
	input.Category = strings.TrimSpace(input.Category)
	if n := len([]rune(input.Description)); n < 1 || n > 200 {
		return Transaction{}, ErrInvalidDescription
	}
	if n := len([]rune(input.Category)); n < 1 || n > 80 {
		return Transaction{}, ErrInvalidCategory
	}
	if (input.Kind == "income" && input.PaymentMethod != "") ||
		(input.Kind == "expense" && input.PaymentMethod != "" && input.PaymentMethod != "card" && input.PaymentMethod != "pix" && input.PaymentMethod != "debit") ||
		(input.Kind == "expense" && input.PaymentMethod == "" && !legacy) {
		return Transaction{}, ErrInvalidPaymentMethod
	}
	if input.Installments < 1 || input.Installments > 60 ||
		(input.Installments > 1 && input.PaymentMethod != "card") ||
		(input.Kind == "income" && input.Installments != 1) {
		return Transaction{}, ErrInvalidInstallments
	}
	if input.OccurredAt.IsZero() {
		return Transaction{}, ErrInvalidOccurredAt
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Transaction{}, err
	}
	return Transaction{
		ID: id, UserID: input.UserID, AmountMinor: input.AmountMinor,
		NecessityLevel: input.NecessityLevel, Kind: input.Kind,
		Description: input.Description, Category: input.Category,
		PaymentMethod: input.PaymentMethod, Installments: input.Installments,
		OccurredAt: input.OccurredAt.UTC(), CreatedAt: time.Now().UTC(),
	}, nil
}
