package model

import (
	"fmt"
	"strings"
)

// Verdict classifies how a claim relates to the knowledge base's evidence
// for it, as decided by a ClaimJudge. InsufficientEvidence is distinct from
// every other value specifically because it must never be read as "the
// claim is false" — it only means the knowledge base had nothing relevant
// to check it against.
type Verdict int

const (
	// VerdictSupported means the cited evidence backs the claim.
	VerdictSupported Verdict = iota

	// VerdictContradicted means the cited evidence directly contradicts the claim.
	VerdictContradicted

	// VerdictPartiallySupported means the cited evidence is relevant but
	// qualified, conditional, or incomplete relative to the claim as stated
	// (e.g. true only under specific conditions the claim omits).
	VerdictPartiallySupported

	// VerdictMixed means the cited evidence genuinely conflicts — some of it
	// supports the claim, some contradicts it.
	VerdictMixed

	// VerdictInsufficientEvidence means no relevant evidence was found in the
	// knowledge base. Never implies the claim is right or wrong.
	VerdictInsufficientEvidence
)

// verdictNames is the canonical string form of each Verdict, used both for
// String() and ParseVerdict — the wire format an LLM judge response
// communicates verdicts in.
var verdictNames = map[Verdict]string{
	VerdictSupported:            "supported",
	VerdictContradicted:         "contradicted",
	VerdictPartiallySupported:   "partially_supported",
	VerdictMixed:                "mixed",
	VerdictInsufficientEvidence: "insufficient_evidence",
}

// String returns v's canonical lowercase name, or "insufficient_evidence"
// for an out-of-range value — the safest fallback given InsufficientEvidence
// never implies a claim is right or wrong.
func (v Verdict) String() string {
	if name, ok := verdictNames[v]; ok {
		return name
	}
	return "insufficient_evidence"
}

// ParseVerdict parses s (case-insensitive) into a Verdict, matching the names
// String() produces. Returns an error for anything else.
func ParseVerdict(s string) (Verdict, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for verdict, name := range verdictNames {
		if name == s {
			return verdict, nil
		}
	}
	return VerdictInsufficientEvidence, fmt.Errorf("unrecognized verdict %q", s)
}

// verdictEmoji is the color-coding shown alongside each Verdict in a
// human-facing rendered message (e.g. a chat bot reply).
var verdictEmoji = map[Verdict]string{
	VerdictSupported:            "✅",
	VerdictContradicted:         "❌",
	VerdictPartiallySupported:   "⚠️",
	VerdictMixed:                "🔶",
	VerdictInsufficientEvidence: "❓",
}

// Emoji returns v's color-coding emoji, or "❓" for an out-of-range value —
// the same safest-fallback reasoning as String().
func (v Verdict) Emoji() string {
	if emoji, ok := verdictEmoji[v]; ok {
		return emoji
	}
	return "❓"
}

// Severity classifies how urgent/high-stakes a claim's subject matter is,
// as decided by a ClaimJudge — used to decide whether to attach a
// recommend-a-real-doctor disclaimer to the response, not to influence the Verdict itself.
type Severity int

const (
	// SeverityRoutine is ordinary informational content.
	SeverityRoutine Severity = iota

	// SeveritySerious covers conditions that warrant professional medical
	// attention but aren't immediately life-threatening.
	SeveritySerious

	// SeverityEmergency covers life-threatening situations — the judge must
	// always recommend consulting a real doctor for these.
	SeverityEmergency
)

// severityNames is the canonical string form of each Severity, used both for
// String() and ParseSeverity — the wire format an LLM judge response
// communicates severity in.
var severityNames = map[Severity]string{
	SeverityRoutine:   "routine",
	SeveritySerious:   "serious",
	SeverityEmergency: "emergency",
}

// String returns s's canonical lowercase name, or "routine" for an
// out-of-range value.
func (s Severity) String() string {
	if name, ok := severityNames[s]; ok {
		return name
	}
	return "routine"
}

// ParseSeverity parses s (case-insensitive) into a Severity, matching the
// names String() produces. Returns an error for anything else.
func ParseSeverity(s string) (Severity, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for severity, name := range severityNames {
		if name == s {
			return severity, nil
		}
	}
	return SeverityRoutine, fmt.Errorf("unrecognized severity %q", s)
}
