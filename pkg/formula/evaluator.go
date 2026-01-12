package formula

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/linagora/qsos-lng/pkg/database"
)

// Evaluate evaluates a formula and returns the result
func (e *Evaluator) Evaluate(ctx context.Context, formula string) (float64, error) {
	// Create a metric provider that reads from the database
	provider := &dbMetricProvider{
		db:         e.db,
		lookup:     e.lookup,
		softwareID: e.softwareID,
	}

	// Parse and evaluate the formula
	tokens := tokenize(formula)
	parser := &parser{tokens: tokens, pos: 0, provider: provider, ctx: ctx}
	return parser.parseExpression()
}

// dbMetricProvider implements MetricProvider using the database
type dbMetricProvider struct {
	db         *database.DB
	lookup     *database.MetricLookup
	softwareID int64
}

// GetMetric retrieves the latest metric value from the database
func (p *dbMetricProvider) GetMetric(ctx context.Context, slug string) (float64, error) {
	metricID, err := p.lookup.GetMetricID(slug)
	if err != nil {
		return 0, err
	}

	var value float64
	err = p.db.Conn.QueryRow(ctx, `
		SELECT value
		FROM categories_metricvalue
		WHERE metric_id = $1 AND software_id = $2
		ORDER BY collected_at DESC
		LIMIT 1
	`, metricID, p.softwareID).Scan(&value)

	if err != nil {
		return 0, fmt.Errorf("failed to get metric '%s': %w", slug, err)
	}

	return value, nil
}

// Token types
type tokenType int

const (
	tokenNumber tokenType = iota
	tokenString
	tokenIdentifier
	tokenOperator
	tokenComma
	tokenLeftParen
	tokenRightParen
	tokenLeftBracket
	tokenRightBracket
	tokenDot
	tokenEOF
)

type token struct {
	typ   tokenType
	value string
}

// tokenize breaks a formula into tokens
func tokenize(formula string) []token {
	var tokens []token
	i := 0
	formula = strings.TrimSpace(formula)

	for i < len(formula) {
		ch := rune(formula[i])

		// Skip whitespace
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// Numbers (including decimals)
		if unicode.IsDigit(ch) || (ch == '-' && i+1 < len(formula) && unicode.IsDigit(rune(formula[i+1]))) {
			start := i
			if ch == '-' {
				i++
			}
			for i < len(formula) && (unicode.IsDigit(rune(formula[i])) || formula[i] == '.') {
				i++
			}
			tokens = append(tokens, token{typ: tokenNumber, value: formula[start:i]})
			continue
		}

		// Identifiers (letters, underscore)
		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			for i < len(formula) && (unicode.IsLetter(rune(formula[i])) || unicode.IsDigit(rune(formula[i])) || formula[i] == '_') {
				i++
			}
			tokens = append(tokens, token{typ: tokenIdentifier, value: formula[start:i]})
			continue
		}

		// String literals
		if ch == '"' || ch == '\'' {
			quote := ch
			i++
			start := i
			for i < len(formula) && rune(formula[i]) != quote {
				i++
			}
			if i >= len(formula) {
				tokens = append(tokens, token{typ: tokenString, value: formula[start:]})
				break
			}
			tokens = append(tokens, token{typ: tokenString, value: formula[start:i]})
			i++ // Skip closing quote
			continue
		}

		// Operators (including comparison operators)
		if strings.HasPrefix(formula[i:], ">=") {
			tokens = append(tokens, token{typ: tokenOperator, value: ">="})
			i += 2
			continue
		}
		if strings.HasPrefix(formula[i:], "<=") {
			tokens = append(tokens, token{typ: tokenOperator, value: "<="})
			i += 2
			continue
		}
		if strings.HasPrefix(formula[i:], "==") {
			tokens = append(tokens, token{typ: tokenOperator, value: "=="})
			i += 2
			continue
		}
		if strings.HasPrefix(formula[i:], "!=") {
			tokens = append(tokens, token{typ: tokenOperator, value: "!="})
			i += 2
			continue
		}

		// Single-character tokens
		switch ch {
		case '+', '-', '*', '/', '>', '<':
			tokens = append(tokens, token{typ: tokenOperator, value: string(ch)})
		case ',':
			tokens = append(tokens, token{typ: tokenComma, value: string(ch)})
		case '(':
			tokens = append(tokens, token{typ: tokenLeftParen, value: string(ch)})
		case ')':
			tokens = append(tokens, token{typ: tokenRightParen, value: string(ch)})
		case '[':
			tokens = append(tokens, token{typ: tokenLeftBracket, value: string(ch)})
		case ']':
			tokens = append(tokens, token{typ: tokenRightBracket, value: string(ch)})
		case '.':
			tokens = append(tokens, token{typ: tokenDot, value: string(ch)})
		}
		i++
	}

	tokens = append(tokens, token{typ: tokenEOF, value: ""})
	return tokens
}

// parser implements a recursive descent parser for formulas
type parser struct {
	tokens   []token
	pos      int
	provider MetricProvider
	ctx      context.Context
}

func (p *parser) current() token {
	if p.pos >= len(p.tokens) {
		return token{typ: tokenEOF, value: ""}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() {
	p.pos++
}

func (p *parser) expect(typ tokenType) error {
	if p.current().typ != typ {
		return fmt.Errorf("expected token type %d, got %d", typ, p.current().typ)
	}
	p.advance()
	return nil
}

// parseExpression parses a full expression (handles comparisons, arithmetic, etc.)
func (p *parser) parseExpression() (float64, error) {
	return p.parseComparison()
}

// parseComparison handles comparison operators (>, <, >=, <=, ==, !=)
func (p *parser) parseComparison() (float64, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return 0, err
	}

	for p.current().typ == tokenOperator {
		op := p.current().value
		if op != ">" && op != "<" && op != ">=" && op != "<=" && op != "==" && op != "!=" {
			break
		}
		p.advance()

		right, err := p.parseAddSub()
		if err != nil {
			return 0, err
		}

		// Return 1.0 for true, 0.0 for false (can be used in if() function)
		var result bool
		switch op {
		case ">":
			result = left > right
		case "<":
			result = left < right
		case ">=":
			result = left >= right
		case "<=":
			result = left <= right
		case "==":
			result = left == right
		case "!=":
			result = left != right
		}

		if result {
			left = 1.0
		} else {
			left = 0.0
		}
	}

	return left, nil
}

// parseAddSub handles addition and subtraction
func (p *parser) parseAddSub() (float64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}

	for p.current().typ == tokenOperator && (p.current().value == "+" || p.current().value == "-") {
		op := p.current().value
		p.advance()

		right, err := p.parseMulDiv()
		if err != nil {
			return 0, err
		}

		if op == "+" {
			left = left + right
		} else {
			left = left - right
		}
	}

	return left, nil
}

// parseMulDiv handles multiplication and division
func (p *parser) parseMulDiv() (float64, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return 0, err
	}

	for p.current().typ == tokenOperator && (p.current().value == "*" || p.current().value == "/") {
		op := p.current().value
		p.advance()

		right, err := p.parsePrimary()
		if err != nil {
			return 0, err
		}

		if op == "*" {
			left = left * right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left = left / right
		}
	}

	return left, nil
}

// parsePrimary handles primary expressions (numbers, function calls, metric references, parentheses)
func (p *parser) parsePrimary() (float64, error) {
	tok := p.current()

	// Numbers
	if tok.typ == tokenNumber {
		p.advance()
		val, err := strconv.ParseFloat(tok.value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number: %s", tok.value)
		}
		return val, nil
	}

	// Parentheses
	if tok.typ == tokenLeftParen {
		p.advance()
		val, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if err := p.expect(tokenRightParen); err != nil {
			return 0, err
		}
		return val, nil
	}

	// Identifiers (functions or metric references)
	if tok.typ == tokenIdentifier {
		name := tok.value
		p.advance()

		// Check if it's a function call
		if p.current().typ == tokenLeftParen {
			return p.parseFunctionCall(name)
		}

		// Check if it's a metric reference (metric.slug)
		if name == "metric" && p.current().typ == tokenDot {
			p.advance() // Skip dot
			if p.current().typ != tokenIdentifier {
				return 0, fmt.Errorf("expected metric slug after 'metric.'")
			}
			slug := p.current().value
			p.advance()
			return p.provider.GetMetric(p.ctx, slug)
		}

		return 0, fmt.Errorf("unexpected identifier: %s", name)
	}

	return 0, fmt.Errorf("unexpected token: %v", tok)
}

// parseFunctionCall parses a function call
func (p *parser) parseFunctionCall(name string) (float64, error) {
	if err := p.expect(tokenLeftParen); err != nil {
		return 0, err
	}

	switch name {
	case "compute_score":
		return p.parseComputeScore()
	case "weighted_avg":
		return p.parseWeightedAvg()
	case "if":
		return p.parseIf()
	default:
		return 0, fmt.Errorf("unknown function: %s", name)
	}
}

// parseComputeScore parses: compute_score(value, [t1, t2, t3, t4], 'direction')
func (p *parser) parseComputeScore() (float64, error) {
	// Parse value
	value, err := p.parseExpression()
	if err != nil {
		return 0, err
	}

	if err := p.expect(tokenComma); err != nil {
		return 0, err
	}

	// Parse thresholds array [t1, t2, t3, t4]
	if err := p.expect(tokenLeftBracket); err != nil {
		return 0, err
	}

	var thresholds [4]float64
	for i := 0; i < 4; i++ {
		thresholds[i], err = p.parseExpression()
		if err != nil {
			return 0, err
		}
		if i < 3 {
			if err := p.expect(tokenComma); err != nil {
				return 0, err
			}
		}
	}

	if err := p.expect(tokenRightBracket); err != nil {
		return 0, err
	}

	if err := p.expect(tokenComma); err != nil {
		return 0, err
	}

	// Parse direction string
	if p.current().typ != tokenString {
		return 0, fmt.Errorf("expected direction string")
	}
	direction := Direction(p.current().value)
	p.advance()

	if err := p.expect(tokenRightParen); err != nil {
		return 0, err
	}

	return ComputeScore(value, thresholds, direction), nil
}

// parseWeightedAvg parses: weighted_avg([v1, v2, ...], total_weight)
func (p *parser) parseWeightedAvg() (float64, error) {
	// Parse values array
	if err := p.expect(tokenLeftBracket); err != nil {
		return 0, err
	}

	var values []float64
	for {
		val, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		values = append(values, val)

		if p.current().typ == tokenRightBracket {
			break
		}
		if err := p.expect(tokenComma); err != nil {
			return 0, err
		}
	}

	if err := p.expect(tokenRightBracket); err != nil {
		return 0, err
	}

	if err := p.expect(tokenComma); err != nil {
		return 0, err
	}

	// Parse total weight
	totalWeight, err := p.parseExpression()
	if err != nil {
		return 0, err
	}

	if err := p.expect(tokenRightParen); err != nil {
		return 0, err
	}

	return WeightedAvg(values, totalWeight)
}

// parseIf parses: if(condition, true_value, false_value)
func (p *parser) parseIf() (float64, error) {
	// Parse condition (comparison expression)
	condition, err := p.parseExpression()
	if err != nil {
		return 0, err
	}

	if err := p.expect(tokenComma); err != nil {
		return 0, err
	}

	// Parse true value
	trueValue, err := p.parseExpression()
	if err != nil {
		return 0, err
	}

	if err := p.expect(tokenComma); err != nil {
		return 0, err
	}

	// Parse false value
	falseValue, err := p.parseExpression()
	if err != nil {
		return 0, err
	}

	if err := p.expect(tokenRightParen); err != nil {
		return 0, err
	}

	// Condition is true if non-zero
	return If(condition != 0, trueValue, falseValue), nil
}
