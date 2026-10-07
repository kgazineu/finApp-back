// Package dataexport exporta todos os dados de um usuário num arquivo JSON e importa esse
// arquivo (outro servidor/banco, uma conta nova ou um backup). Conta com dados só recebe a
// importação com replace=true, que troca tudo pelo conteúdo do arquivo. Os ids do banco não
// viajam: na importação tudo ganha id novo e as referências (conta do lançamento, pai da
// parcela) são refeitas. Datas de criação, arquivamento e pagamento são preservadas.
package dataexport

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/api"
)

const (
	Format  = "finapp-export"
	Version = 1
	// ponytail: limite fixo; histórico de anos de uso pessoal fica bem abaixo disso
	maxImportBytes = 20 << 20
)

var (
	ErrNotEmpty    = errors.New("você já tem dados cadastrados: para trocar tudo pelo conteúdo do arquivo, envie de novo com replace=true")
	ErrInvalidFile = errors.New("arquivo de exportação inválido")
)

// Document é o arquivo exportado. Datas sem hora (DATE) vão como "AAAA-MM-DD".
type Document struct {
	Format                string                 `json:"format"`
	Version               int                    `json:"version"`
	ExportedAt            time.Time              `json:"exportedAt"`
	Accounts              []Account              `json:"accounts"`
	BillingRegistrations  []BillingRegistration  `json:"billingRegistrations"`
	RecurringTransactions []RecurringTransaction `json:"recurringTransactions"`
	Receivables           []Receivable           `json:"receivables"`
	Transactions          []Transaction          `json:"transactions"`
	Goals                 []Goal                 `json:"goals"`
	SavingsGoal           *SavingsGoal           `json:"savingsGoal"` // null sem meta (e em arquivos antigos)
}

type Account struct {
	// ID só liga os lançamentos de saldo à conta dentro do arquivo; não é reaproveitado
	ID         int64      `json:"id" db:"id"`
	Name       string     `json:"name" db:"name"`
	Kind       string     `json:"kind" db:"kind"`
	HasYield   bool       `json:"hasYield" db:"has_yield"`
	ArchivedAt *time.Time `json:"archivedAt" db:"archived_at"`
	CreatedAt  time.Time  `json:"createdAt" db:"created_at"`
}

type BillingRegistration struct {
	ID        int64          `json:"-" db:"id"`
	Delta     *int64         `json:"delta" db:"delta"` // null no primeiro registro
	Total     int64          `json:"total" db:"total"`
	CreatedAt time.Time      `json:"createdAt" db:"created_at"`
	Entries   []BillingEntry `json:"entries" db:"-"`
}

type BillingEntry struct {
	RegistrationID int64  `json:"-" db:"billing_registration_id"`
	AccountID      int64  `json:"accountId" db:"account_id"`
	AccountName    string `json:"accountName" db:"account_name"`
	Amount         int64  `json:"amount" db:"amount"`
}

type RecurringTransaction struct {
	ID             int64         `json:"-" db:"id"`
	Description    string        `json:"description" db:"description"`
	Kind           string        `json:"kind" db:"kind"`
	IsFixed        bool          `json:"isFixed" db:"is_fixed"`
	Amount         int64         `json:"amount" db:"amount"`
	StartMonth     string        `json:"startMonth" db:"start_month"`
	IntervalMonths int           `json:"intervalMonths" db:"interval_months"`
	EndMonth       *string       `json:"endMonth" db:"end_month"`
	DayOfMonth     *int          `json:"dayOfMonth" db:"day_of_month"`
	ArchivedAt     *time.Time    `json:"archivedAt" db:"archived_at"`
	CreatedAt      time.Time     `json:"createdAt" db:"created_at"`
	Installments   []Installment `json:"installments" db:"-"`
}

type Receivable struct {
	ID           int64         `json:"-" db:"id"`
	Kind         string        `json:"kind" db:"kind"`
	Debtor       string        `json:"debtor" db:"debtor"`
	Description  string        `json:"description" db:"description"`
	Amount       int64         `json:"amount" db:"amount"`
	AmountMode   string        `json:"amountMode" db:"amount_mode"` // ausente em arquivos antigos = "total"
	InterestRate int           `json:"interestRate" db:"interest_rate"`
	ArchivedAt   *time.Time    `json:"archivedAt" db:"archived_at"`
	CreatedAt    time.Time     `json:"createdAt" db:"created_at"`
	Installments []Installment `json:"installments" db:"-"`
}

type Installment struct {
	ParentID int64      `json:"-" db:"parent_id"`
	Number   int        `json:"number" db:"number"`
	Amount   int64      `json:"amount" db:"amount"`
	DueDate  string     `json:"dueDate" db:"due_date"`
	PaidAt   *time.Time `json:"paidAt" db:"paid_at"`
}

// Transaction é o lançamento do histórico (POST /transactions).
type Transaction struct {
	Kind           string    `json:"kind" db:"kind"`
	AmountMinor    int64     `json:"amountMinor" db:"amount_minor"`
	NecessityLevel *int      `json:"necessityLevel" db:"necessity_level"`
	Description    string    `json:"description" db:"description"`
	Category       string    `json:"category" db:"category"`
	PaymentMethod  *string   `json:"paymentMethod" db:"payment_method"`
	Installments   int       `json:"installments" db:"installments"`
	OccurredAt     time.Time `json:"occurredAt" db:"occurred_at"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
}

// SavingsGoal é a meta de guardar por mês (savings_goals).
type SavingsGoal struct {
	Percent *int   `json:"percent" db:"percent"`
	Amount  *int64 `json:"amount" db:"amount"`
}

type Goal struct {
	Name        string    `json:"name" db:"name"`
	TargetMinor int64     `json:"targetMinor" db:"target_minor"`
	SavedMinor  int64     `json:"savedMinor" db:"saved_minor"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

type Module struct {
	repo *Repository
}

func NewModule(db *sqlx.DB) *Module {
	return &Module{repo: NewRepository(db)}
}

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/export", m.export)
	r.POST("/import", m.importFile)
}

func (m *Module) export(c *gin.Context) {
	doc, err := m.repo.Export(c.Request.Context(), api.UserID(c))
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="finapp-dados-%s.json"`, doc.ExportedAt.Format(time.DateOnly)))
	c.Header("Cache-Control", "no-store") // dados pessoais completos: nem o cache da API nem o navegador guardam
	c.JSON(http.StatusOK, doc)
}

func (m *Module) importFile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportBytes)

	var doc Document
	if err := c.ShouldBindJSON(&doc); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": "Arquivo maior que 20 MB"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": ErrInvalidFile.Error() + ": não é um JSON válido"})
		return
	}
	if doc.Format != Format || doc.Version != Version {
		c.JSON(http.StatusBadRequest, gin.H{"message": ErrInvalidFile.Error() + ": não foi gerado pela exportação do FinApp"})
		return
	}

	summary, err := m.repo.Import(c.Request.Context(), api.UserID(c), &doc, c.Query("replace") == "true")
	if errors.Is(err, ErrNotEmpty) {
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		return
	}
	if errors.Is(err, ErrInvalidFile) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": summary})
}
