package model

import "testing"

func TestVerdict_StringAndParseRoundTrip(t *testing.T) {
	verdicts := []Verdict{
		VerdictSupported,
		VerdictContradicted,
		VerdictPartiallySupported,
		VerdictMixed,
		VerdictInsufficientEvidence,
	}
	for _, v := range verdicts {
		got, err := ParseVerdict(v.String())
		if err != nil {
			t.Errorf("ParseVerdict(%q) returned unexpected error: %v", v.String(), err)
		}
		if got != v {
			t.Errorf("round-trip mismatch: %v -> %q -> %v", v, v.String(), got)
		}
	}
}

func TestParseVerdict_UnrecognizedDefaultsToInsufficientEvidence(t *testing.T) {
	got, err := ParseVerdict("not-a-real-verdict")
	if err == nil {
		t.Error("expected an error for an unrecognized verdict")
	}
	if got != VerdictInsufficientEvidence {
		t.Errorf(
			"expected the safe default VerdictInsufficientEvidence, got %v — "+
				"this default matters: it must never silently read as a confident verdict",
			got,
		)
	}
}

func TestParseVerdict_CaseInsensitiveAndTrimmed(t *testing.T) {
	got, err := ParseVerdict("  Supported  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != VerdictSupported {
		t.Errorf("expected VerdictSupported, got %v", got)
	}
}

func TestSeverity_StringAndParseRoundTrip(t *testing.T) {
	severities := []Severity{SeverityRoutine, SeveritySerious, SeverityEmergency}
	for _, s := range severities {
		got, err := ParseSeverity(s.String())
		if err != nil {
			t.Errorf("ParseSeverity(%q) returned unexpected error: %v", s.String(), err)
		}
		if got != s {
			t.Errorf("round-trip mismatch: %v -> %q -> %v", s, s.String(), got)
		}
	}
}

func TestParseSeverity_Unrecognized(t *testing.T) {
	if _, err := ParseSeverity("not-a-real-severity"); err == nil {
		t.Error("expected an error for an unrecognized severity")
	}
}
