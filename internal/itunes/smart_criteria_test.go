// file: internal/itunes/smart_criteria_test.go
// version: 1.0.0
// guid: 0d8e9f7a-1b2c-4a70-b8c5-3d7e0f1b9a99

package itunes

import (
	"encoding/binary"
	"testing"
)

func buildMockRule(field SmartField, op SmartOperator, strVal string) []byte {
	data := make([]byte, 136)
	binary.LittleEndian.PutUint32(data[0:4], uint32(field))
	binary.LittleEndian.PutUint32(data[4:8], uint32(op))

	if strVal != "" {
		utf16 := make([]byte, len(strVal)*2)
		for i, r := range strVal {
			binary.LittleEndian.PutUint16(utf16[i*2:], uint16(r))
		}
		binary.LittleEndian.PutUint32(data[52:56], uint32(len(strVal)))
		copy(data[56:], utf16)
	}
	return data
}

func buildMockCriteria(conjunction byte, rules ...[]byte) []byte {
	header := make([]byte, 8)
	header[4] = conjunction // 1=AND, 0=OR
	var blob []byte
	blob = append(blob, header...)
	for _, r := range rules {
		blob = append(blob, r...)
	}
	return blob
}

func TestParseSmartCriteria_SingleRule(t *testing.T) {
	rule := buildMockRule(SmartFieldArtist, SmartOpContains, "Sanderson")
	blob := buildMockCriteria(1, rule) // AND

	result, err := ParseSmartCriteria(blob)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Conjunction != "AND" {
		t.Errorf("conjunction = %q, want AND", result.Conjunction)
	}
	if len(result.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(result.Rules))
	}
	if result.Rules[0].Field != SmartFieldArtist {
		t.Errorf("field = %v", result.Rules[0].Field)
	}
	if result.Rules[0].Operator != SmartOpContains {
		t.Errorf("operator = %v", result.Rules[0].Operator)
	}
	if len(result.Rules[0].Operands) == 0 || result.Rules[0].Operands[0] != "Sanderson" {
		t.Errorf("operands = %v", result.Rules[0].Operands)
	}
}

func TestParseSmartCriteria_ORConjunction(t *testing.T) {
	r1 := buildMockRule(SmartFieldGenre, SmartOpIs, "Fiction")
	r2 := buildMockRule(SmartFieldGenre, SmartOpIs, "SciFi")
	blob := buildMockCriteria(0, r1, r2) // OR

	result, err := ParseSmartCriteria(blob)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Conjunction != "OR" {
		t.Errorf("conjunction = %q, want OR", result.Conjunction)
	}
	if len(result.Rules) != 2 {
		t.Errorf("rules = %d, want 2", len(result.Rules))
	}
}

func TestTranslateSmartCriteria_Basic(t *testing.T) {
	parsed := &SmartCriteriaResult{
		Conjunction: "AND",
		Rules: []SmartRule{
			{Field: SmartFieldArtist, Operator: SmartOpContains, Operands: []string{"Sanderson"}},
			{Field: SmartFieldYear, Operator: SmartOpGreaterThan, Operands: []string{"2015"}},
		},
	}

	dsl := TranslateSmartCriteria(parsed)
	if dsl != "author:*Sanderson* year:>2015" {
		t.Errorf("dsl = %q", dsl)
	}
}

func TestTranslateSmartCriteria_OR(t *testing.T) {
	parsed := &SmartCriteriaResult{
		Conjunction: "OR",
		Rules: []SmartRule{
			{Field: SmartFieldGenre, Operator: SmartOpIs, Operands: []string{"Fiction"}},
			{Field: SmartFieldGenre, Operator: SmartOpIs, Operands: []string{"SciFi"}},
		},
	}

	dsl := TranslateSmartCriteria(parsed)
	if dsl != "genre:Fiction || genre:SciFi" {
		t.Errorf("dsl = %q", dsl)
	}
}

func TestTranslateSmartCriteria_Negation(t *testing.T) {
	parsed := &SmartCriteriaResult{
		Conjunction: "AND",
		Rules: []SmartRule{
			{Field: SmartFieldArtist, Operator: SmartOpIsNot, Operands: []string{"Unknown"}},
		},
	}

	dsl := TranslateSmartCriteria(parsed)
	if dsl != "-author:Unknown" {
		t.Errorf("dsl = %q", dsl)
	}
}

func TestParseSmartCriteria_TooShort(t *testing.T) {
	_, err := ParseSmartCriteria([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error on short blob")
	}
}
