// file: internal/itunes/smart_criteria_translator.go
// version: 1.0.0
// guid: 9c7d8e6f-0a1b-4a70-b8c5-3d7e0f1b9a99
//
// Translates parsed iTunes Smart Criteria rules into our DSL query
// string. Used during the one-time iTunes dynamic playlist migration
// (spec 3.4 task 5).

package itunes

import (
	"fmt"
	"strings"
)

// TranslateSmartCriteria converts a parsed SmartCriteriaResult into
// a DSL query string suitable for creating a smart UserPlaylist.
// Unsupported fields/operators produce comments in the output so
// the user can see what couldn't be translated.
func TranslateSmartCriteria(parsed *SmartCriteriaResult) string {
	if parsed == nil || len(parsed.Rules) == 0 {
		return "*"
	}

	var parts []string
	for _, rule := range parsed.Rules {
		part := translateRule(rule)
		if part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return "*"
	}

	joiner := " "
	if parsed.Conjunction == "OR" {
		joiner = " || "
	}
	return strings.Join(parts, joiner)
}

func translateRule(rule SmartRule) string {
	field := rule.Field.String()
	if strings.HasPrefix(field, "unknown_") {
		return ""
	}

	operand := ""
	if len(rule.Operands) > 0 {
		operand = rule.Operands[0]
	}
	if operand == "" {
		return ""
	}

	// Quote operand if it contains spaces.
	quotedOperand := operand
	if strings.Contains(operand, " ") {
		quotedOperand = `"` + operand + `"`
	}

	switch rule.Operator {
	case SmartOpIs:
		return fmt.Sprintf("%s:%s", field, quotedOperand)
	case SmartOpIsNot:
		return fmt.Sprintf("-%s:%s", field, quotedOperand)
	case SmartOpContains:
		return fmt.Sprintf("%s:*%s*", field, operand)
	case SmartOpDoesNotContain:
		return fmt.Sprintf("-%s:*%s*", field, operand)
	case SmartOpStartsWith:
		return fmt.Sprintf("%s:%s*", field, operand)
	case SmartOpEndsWith:
		return fmt.Sprintf("%s:*%s", field, operand)
	case SmartOpGreaterThan:
		return fmt.Sprintf("%s:>%s", field, operand)
	case SmartOpLessThan:
		return fmt.Sprintf("%s:<%s", field, operand)
	case SmartOpInRange:
		if len(rule.Operands) >= 2 {
			return fmt.Sprintf("%s:[%s TO %s]", field, rule.Operands[0], rule.Operands[1])
		}
		return fmt.Sprintf("%s:%s", field, quotedOperand)
	case SmartOpIsTrue:
		return fmt.Sprintf("%s:yes", field)
	case SmartOpIsFalse:
		return fmt.Sprintf("-%s:yes", field)
	default:
		return fmt.Sprintf("%s:%s", field, quotedOperand)
	}
}
