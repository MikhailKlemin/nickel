package statement

import (
	"bufio"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const dateLayoutFR = "02/01/2006"

var (
	txStart = regexp.MustCompile(`^\s{1,12}\d+\s{2,}\d{2}/\d{2}/\d{4}\s`)

	pageBreak = regexp.MustCompile(`\f|\b\d+/\d+\b$|OPÉRATIONS DÉBIT DIFFÉRÉ|Sous réserve|RELEVE DE COMPTE NICKEL`)

	reAmount = regexp.MustCompile(`\s+(-?\d[\d\s]*,\d{2}\s*€?)\s*$`)
	reHead   = regexp.MustCompile(`^\s*(\d+)\s+(\d{2}/\d{2}/\d{4})\s+`)

	opTypes = []string{
		"FRAIS RETRAIT DAB",
		"RETRAIT DAB",
		"PRELEVEMENT",
		"VIREMENT",
		"ACHAT",
	}

	reInternalSpaces = regexp.MustCompile(`(\d)\s+(\d)`)

	reIBAN   = regexp.MustCompile(`IBAN\s*:\s*(FR\w+)`)
	reBIC    = regexp.MustCompile(`BIC\s*:\s*(\w+)`)
	rePeriod = regexp.MustCompile(`Du\s+(\d{2}/\d{2}/\d{4})\s+au\s+(\d{2}/\d{2}/\d{4})`)
	reAccNum = regexp.MustCompile(`Numéro de compte\s*:\s*(\d+)`)
	reHolder = regexp.MustCompile(`(?:M|MME|MLLE)\s+[A-ZÀÂÄÉÈÊËÎÏÔÙÛÜ][A-ZÀÂÄÉÈÊËÎÏÔÙÛÜ\s]+`)

	reDigitsOnly = regexp.MustCompile(`^[\d\s-]+$`)
	reIBANLike   = regexp.MustCompile(`^FR\d[\dA-Z]{10,}$`)
)

type anyLogger interface {
	Warn(msg string, args ...any)
}

type rawBlock struct {
	Leading  []string
	Row      string
	Trailing []string
}

func Parse(text string, logger anyLogger) (Statement, error) {
	holder, accNum, iban, bic, from, to := parseHeader(text)

	transactions, err := parseTransactions(text, logger)
	if err != nil {
		return Statement{}, fmt.Errorf("parse transactions: %w", err)
	}

	return Statement{
		AccountHolder: holder,
		AccountNumber: accNum,
		IBAN:          iban,
		BIC:           bic,
		PeriodFrom:    from,
		PeriodTo:      to,
		Transactions:  transactions,
	}, nil
}

func parseHeader(text string) (holder, accNum, iban, bic, from, to string) {
	if m := reIBAN.FindStringSubmatch(text); m != nil {
		iban = m[1]
	}
	if m := reBIC.FindStringSubmatch(text); m != nil {
		bic = m[1]
	}
	if m := rePeriod.FindStringSubmatch(text); m != nil {
		from, to = m[1], m[2]
	}
	if m := reAccNum.FindStringSubmatch(text); m != nil {
		accNum = m[1]
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "TITULAIRE DU COMPTE") {
			continue
		}
		for _, candidate := range lines[i+1 : min(i+6, len(lines))] {
			if m := reHolder.FindStringSubmatch(candidate); m != nil {
				holder = strings.TrimSpace(m[0])
				return
			}
		}
	}

	return
}

func SplitTransactions(text string) ([]string, error) {
	var transactions []string
	var current *rawBlock
	var pending []string

	saveCurrent := func() {
		if current == nil {
			return
		}
		if raw := strings.TrimSpace(current.String()); raw != "" {
			transactions = append(transactions, raw)
		}
		current = nil
	}

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()

		if pageBreak.MatchString(line) {
			if current != nil {
				current.Trailing = append(current.Trailing, pending...)
				pending = nil
				saveCurrent()
			} else {
				pending = nil
			}
			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		if txStart.MatchString(line) {
			if current == nil {
				current = &rawBlock{Row: line}
				pending = nil
				continue
			}

			trailing, leading := splitAmbiguousRun(pending)
			current.Trailing = append(current.Trailing, trailing...)
			saveCurrent()

			current = &rawBlock{
				Leading: leading,
				Row:     line,
			}
			pending = nil
			continue
		}

		if current != nil {
			pending = append(pending, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan statement text: %w", err)
	}

	if current != nil {
		current.Trailing = append(current.Trailing, pending...)
		saveCurrent()
	}

	return transactions, nil
}

func (b rawBlock) String() string {
	parts := make([]string, 0, len(b.Leading)+1+len(b.Trailing))

	for _, line := range b.Leading {
		if strings.TrimSpace(line) != "" {
			parts = append(parts, line)
		}
	}
	if strings.TrimSpace(b.Row) != "" {
		parts = append(parts, b.Row)
	}
	for _, line := range b.Trailing {
		if strings.TrimSpace(line) != "" {
			parts = append(parts, line)
		}
	}

	return strings.Join(parts, "\n")
}

func splitAmbiguousRun(lines []string) (trailing, leading []string) {
	if len(lines) == 0 {
		return nil, nil
	}

	cut := len(lines)
	for cut > 0 && looksLikeLeadingLine(lines[cut-1]) {
		cut--
	}

	return slices.Clone(lines[:cut]), slices.Clone(lines[cut:])
}

func looksLikeLeadingLine(line string) bool {
	s := normalizeWhitespace(line)
	if s == "" {
		return false
	}

	compact := strings.ReplaceAll(s, " ", "")
	upper := strings.ToUpper(s)

	switch {
	case reIBANLike.MatchString(compact):
		return false
	case reDigitsOnly.MatchString(s):
		return false
	case strings.HasPrefix(upper, "CONTRAT"):
		return false
	case strings.HasPrefix(upper, "REF"):
		return false
	case strings.HasPrefix(upper, "REFERENCE"):
		return false
	default:
		return true
	}
}

func ParseTransaction(raw string) (Transaction, error) {
	lines := strings.Split(raw, "\n")

	mainIdx := -1
	var tx Transaction

	for i, line := range lines {
		parsed, ok, err := parseMainLine(line)
		if err != nil {
			return Transaction{}, err
		}
		if ok {
			mainIdx = i
			tx = parsed
			break
		}
	}

	if mainIdx == -1 {
		return Transaction{}, fmt.Errorf("no transaction row found in block: %q", raw)
	}

	var descParts []string
	for _, line := range lines[:mainIdx] {
		if part := normalizeDescription(line); part != "" {
			descParts = append(descParts, part)
		}
	}
	if inline := normalizeDescription(tx.Description); inline != "" {
		descParts = append(descParts, inline)
	}
	for _, line := range lines[mainIdx+1:] {
		if part := normalizeDescription(line); part != "" {
			descParts = append(descParts, part)
		}
	}

	tx.Description = strings.Join(descParts, " ")
	return tx, nil
}

func parseMainLine(line string) (Transaction, bool, error) {
	s := normalizeTransactionText(line)
	if s == "" {
		return Transaction{}, false, nil
	}

	amountMatch := reAmount.FindStringSubmatchIndex(s)
	if amountMatch == nil {
		return Transaction{}, false, nil
	}

	rawAmount := normalizeAmount(s[amountMatch[2]:amountMatch[3]])
	amountCents, err := parseAmountCents(rawAmount)
	if err != nil {
		return Transaction{}, false, fmt.Errorf("parse amount %q: %w", rawAmount, err)
	}

	s = strings.TrimSpace(s[:amountMatch[0]])
	head := reHead.FindStringSubmatch(s)
	if head == nil {
		return Transaction{}, false, nil
	}

	num, err := strconv.Atoi(head[1])
	if err != nil {
		return Transaction{}, false, fmt.Errorf("parse transaction number %q: %w", head[1], err)
	}

	dateValue, err := time.Parse(dateLayoutFR, head[2])
	if err != nil {
		return Transaction{}, false, fmt.Errorf("parse transaction date %q: %w", head[2], err)
	}

	rest := strings.TrimSpace(strings.TrimPrefix(s, head[0]))
	for _, op := range opTypes {
		if rest == op || strings.HasPrefix(rest, op+" ") {
			desc := strings.TrimSpace(strings.TrimPrefix(rest, op))
			return Transaction{
				Number:      num,
				Date:        dateValue,
				RawDate:     head[2],
				Type:        op,
				Description: normalizeDescription(desc),
				AmountCents: amountCents,
				RawAmount:   rawAmount,
			}, true, nil
		}
	}

	return Transaction{}, false, nil
}

func parseTransactions(text string, logger anyLogger) ([]Transaction, error) {
	rawTransactions, err := SplitTransactions(text)
	if err != nil {
		return nil, fmt.Errorf("split transactions: %w", err)
	}

	txs := make([]Transaction, 0, len(rawTransactions))
	for _, raw := range rawTransactions {
		tx, err := ParseTransaction(raw)
		if err != nil {
			if logger != nil {
				preview := normalizeWhitespace(raw)
				if len(preview) > 180 {
					preview = preview[:180] + "..."
				}
				logger.Warn("skipping malformed transaction block", slog.String("err", err.Error()), slog.String("preview", preview))
			}
			continue
		}
		txs = append(txs, tx)
	}

	return txs, nil
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func normalizeTransactionText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "*>", " ")
	s = strings.ReplaceAll(s, ">*", " ")
	return normalizeWhitespace(s)
}

func normalizeDescription(s string) string {
	s = normalizeTransactionText(s)
	s = strings.TrimLeft(s, " *>")
	return strings.TrimSpace(s)
}

func normalizeAmount(raw string) string {
	for reInternalSpaces.MatchString(raw) {
		raw = reInternalSpaces.ReplaceAllString(raw, "$1$2")
	}
	return strings.TrimSpace(raw)
}

func parseAmountCents(raw string) (int64, error) {
	s := normalizeAmount(raw)
	s = strings.TrimSuffix(s, "€")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")

	sign := int64(1)
	if strings.HasPrefix(s, "-") {
		sign = -1
		s = strings.TrimPrefix(s, "-")
	}

	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid money format")
	}

	euros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	cents, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, err
	}

	if cents < 0 || cents > 99 {
		return 0, fmt.Errorf("invalid cents component")
	}

	return sign * (euros*100 + cents), nil
}

func formatAmountEuro(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}
