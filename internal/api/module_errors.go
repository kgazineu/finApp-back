package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// fieldLabels names the request fields of the balance modules in Portuguese.
var fieldLabels = map[string]string{
	"Name": "nome", "Kind": "tipo", "HasYield": "rendimento", "Entries": "lançamentos",
	"AccountID": "conta", "Amount": "valor", "Description": "descrição", "IsFixed": "fixa",
	"StartMonth": "mês de início", "IntervalMonths": "intervalo de meses", "Installments": "parcelas",
	"DayOfMonth": "dia do mês", "EndMonth": "mês final", "Paid": "pago", "Debtor": "devedor",
	"InterestRate": "juros", "FirstDueDate": "primeiro vencimento", "Email": "e-mail",
	"Code": "código", "Password": "senha",
}

// BindError translates a Gin binding error (malformed JSON or failed validation) into Portuguese.
func BindError(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return fmt.Sprintf("Tipo inválido no campo %s", typeErr.Field)
	}
	var validation validator.ValidationErrors
	if !errors.As(err, &validation) || len(validation) == 0 {
		return "JSON inválido"
	}

	e := validation[0]
	field := fieldLabels[e.Field()]
	if field == "" {
		field = e.Field()
	}
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("O campo %s é obrigatório", field)
	case "oneof":
		return fmt.Sprintf("O campo %s deve ser um destes: %s", field, strings.ReplaceAll(e.Param(), " ", ", "))
	case "gt":
		return fmt.Sprintf("O campo %s deve ser maior que %s", field, e.Param())
	case "gte":
		return fmt.Sprintf("O campo %s deve ser maior ou igual a %s", field, e.Param())
	case "lte":
		return fmt.Sprintf("O campo %s deve ser menor ou igual a %s", field, e.Param())
	case "min":
		return fmt.Sprintf("O campo %s precisa de pelo menos %s item", field, e.Param())
	case "unique":
		return fmt.Sprintf("O campo %s tem itens repetidos", field)
	case "email":
		return "E-mail inválido"
	}
	return fmt.Sprintf("O campo %s é inválido", field)
}

// InternalError logs the cause and answers 500 without exposing database details.
func InternalError(c *gin.Context, err error) {
	slog.Error("erro interno", "method", c.Request.Method, "path", c.FullPath(), "error", err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Erro interno do servidor, tente novamente"})
}
