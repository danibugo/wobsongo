package model

import "testing"

func TestTruthTier_StringAndParseRoundTrip(t *testing.T) {
	tiers := []TruthTier{
		TruthTierAxiomatic,
		TruthTierTemporal,
		TruthTierProbabilistic,
		TruthTierSubjective,
		TruthTierUnknown,
		TruthTierInvalid,
	}
	for _, tier := range tiers {
		got, err := ParseTruthTier(tier.String())
		if err != nil {
			t.Errorf("ParseTruthTier(%q) returned unexpected error: %v", tier.String(), err)
		}
		if got != tier {
			t.Errorf("round-trip mismatch: %v -> %q -> %v", tier, tier.String(), got)
		}
	}
}

func TestTruthTier_StringOutOfRange(t *testing.T) {
	if got := TruthTier(999).String(); got != unknownLabel {
		t.Errorf("String() for out-of-range value = %q, want %q", got, unknownLabel)
	}
}

func TestParseTruthTier_Unrecognized(t *testing.T) {
	got, err := ParseTruthTier("not-a-real-tier")
	if err == nil {
		t.Error("expected an error for an unrecognized truth tier")
	}
	if got != TruthTierUnknown {
		t.Errorf("expected the safe default TruthTierUnknown, got %v", got)
	}
}

func TestParseTruthTier_CaseInsensitiveAndTrimmed(t *testing.T) {
	got, err := ParseTruthTier("  Axiomatic  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != TruthTierAxiomatic {
		t.Errorf("expected TruthTierAxiomatic, got %v", got)
	}
}

func TestFactCategory_StringAndParseRoundTrip(t *testing.T) {
	categories := []FactCategory{FactCategoryClinical, FactCategoryMetadata, FactCategoryUnknown}
	for _, c := range categories {
		got, err := ParseFactCategory(c.String())
		if err != nil {
			t.Errorf("ParseFactCategory(%q) returned unexpected error: %v", c.String(), err)
		}
		if got != c {
			t.Errorf("round-trip mismatch: %v -> %q -> %v", c, c.String(), got)
		}
	}
}

func TestFactCategory_StringOutOfRange(t *testing.T) {
	if got := FactCategory(999).String(); got != unknownLabel {
		t.Errorf("String() for out-of-range value = %q, want %q", got, unknownLabel)
	}
}

func TestParseFactCategory_Unrecognized(t *testing.T) {
	if _, err := ParseFactCategory("not-a-real-category"); err == nil {
		t.Error("expected an error for an unrecognized fact category")
	}
}

func TestLanguage_StringAndParseRoundTrip(t *testing.T) {
	languages := []Language{LanguageEnglish, LanguageFrench}
	for _, l := range languages {
		got, err := ParseLanguage(l.String())
		if err != nil {
			t.Errorf("ParseLanguage(%q) returned unexpected error: %v", l.String(), err)
		}
		if got != l {
			t.Errorf("round-trip mismatch: %v -> %q -> %v", l, l.String(), got)
		}
	}
}

func TestLanguage_StringOutOfRange(t *testing.T) {
	if got := Language(999).String(); got != "en" {
		t.Errorf("String() for out-of-range value = %q, want %q", got, "en")
	}
}

func TestParseLanguage_Unrecognized(t *testing.T) {
	got, err := ParseLanguage("not-a-real-language")
	if err == nil {
		t.Error("expected an error for an unrecognized language")
	}
	if got != LanguageEnglish {
		t.Errorf("expected the safe default LanguageEnglish, got %v", got)
	}
}

func TestLanguage_Other(t *testing.T) {
	tests := []struct {
		lang Language
		want Language
	}{
		{LanguageEnglish, LanguageFrench},
		{LanguageFrench, LanguageEnglish},
	}
	for _, tt := range tests {
		if got := tt.lang.Other(); got != tt.want {
			t.Errorf("%v.Other() = %v, want %v", tt.lang, got, tt.want)
		}
	}
}

func TestAtomicKnowledge_SPOText(t *testing.T) {
	tests := []struct {
		name string
		fact AtomicKnowledge
		want string
	}{
		{
			name: "without note",
			fact: AtomicKnowledge{Subject: "aspirin", Predicate: "reduces risk of", Object: "heart attack"},
			want: "aspirin reduces risk of heart attack",
		},
		{
			name: "with note",
			fact: AtomicKnowledge{
				Subject: "aspirin", Predicate: "reduces risk of", Object: "heart attack",
				Note: "in adults over 50",
			},
			want: "aspirin reduces risk of heart attack in adults over 50",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fact.SPOText(); got != tt.want {
				t.Errorf("SPOText() = %q, want %q", got, tt.want)
			}
		})
	}
}
