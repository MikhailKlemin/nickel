package statement

import (
	"encoding/json"
	"time"
)

const dateLayoutISO = "2006-01-02"

type Statement struct {
	AccountHolder string        `json:"account_holder"`
	AccountNumber string        `json:"account_number"`
	IBAN          string        `json:"iban"`
	BIC           string        `json:"bic"`
	PeriodFrom    string        `json:"period_from"`
	PeriodTo      string        `json:"period_to"`
	Transactions  []Transaction `json:"transactions"`
}

type Transaction struct {
	Number      int
	Date        time.Time
	RawDate     string
	Type        string
	Description string
	AmountCents int64
	RawAmount   string
}

type transactionJSON struct {
	Number      int    `json:"number"`
	Date        string `json:"date"`
	DateRaw     string `json:"date_raw,omitempty"`
	Type        string `json:"type"`
	Description string `json:"description"`
	AmountCents int64  `json:"amount_cents"`
	AmountEuro  string `json:"amount_eur"`
	AmountRaw   string `json:"amount_raw,omitempty"`
}

func (t Transaction) MarshalJSON() ([]byte, error) {
	payload := transactionJSON{
		Number:      t.Number,
		Date:        t.DateISO(),
		DateRaw:     t.RawDate,
		Type:        t.Type,
		Description: t.Description,
		AmountCents: t.AmountCents,
		AmountEuro:  t.AmountEuroString(),
		AmountRaw:   t.RawAmount,
	}
	return json.Marshal(payload)
}

func (t Transaction) DateISO() string {
	if t.Date.IsZero() {
		return ""
	}
	return t.Date.Format(dateLayoutISO)
}

func (t Transaction) AmountEuroString() string {
	return formatAmountEuro(t.AmountCents)
}
