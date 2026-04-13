//go:build go1.25

package statement

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const dateLayoutISO = "2006-01-02"

type ParsedStatement struct {
	AccountHolder   string              `json:"account_holder"`
	AccountNumber   string              `json:"account_number"`
	IBAN            string              `json:"iban"`
	BIC             string              `json:"bic"`
	PeriodFrom      string              `json:"period_from"`
	PeriodTo        string              `json:"period_to"`
	Transactions    []ParsedTransaction `json:"transactions"`
	SkippedTxBlocks int                 `json:"skipped_tx_blocks"` // blocks the parser could not parse
}

type ParsedTransaction struct {
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

func (t ParsedTransaction) MarshalJSON() ([]byte, error) {
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

func (t ParsedTransaction) DateISO() string {
	if t.Date.IsZero() {
		return ""
	}
	return t.Date.Format(dateLayoutISO)
}

func (t ParsedTransaction) AmountEuroString() string {
	return formatAmountEuro(t.AmountCents)
}

func formatAmountEuro(cents int64) string {
	var b strings.Builder
	b.Grow(12) // Pre-allocate for typical amounts like "-1234.56"
	
	if cents < 0 {
		b.WriteByte('-')
		cents = -cents
	}
	
	euros := cents / 100
	centsRem := cents % 100
	
	fmt.Fprintf(&b, "%d.%02d", euros, centsRem)
	return b.String()
}
