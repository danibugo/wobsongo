package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
)

// judgeMaxTokens bounds the judge's response length — a verdict, severity,
// short reasoning, and a handful of citation indices, not a long generation.
const judgeMaxTokens = 1500

// judgeHTTPTimeout bounds a single judge call.
const judgeHTTPTimeout = 3 * time.Minute

// judgeChatRoleUser is the chat-completion message role for the caller's input.
const judgeChatRoleUser = "user"

// JudgeClient implements data.ClaimJudge against a generic OpenAI-compatible
// text chat-completions API — same shape as ExtractionClient.
type JudgeClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// Ensure JudgeClient implements data.ClaimJudge.
var _ data.ClaimJudge = (*JudgeClient)(nil)

// NewJudgeClient creates a new JudgeClient targeting the given base
// URL/model. apiKey may be empty — self-hosted servers often need no auth.
func NewJudgeClient(baseURL, model, apiKey string) *JudgeClient {
	return &JudgeClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: judgeHTTPTimeout,
		},
	}
}

// judgeVerdictJSON is the wire shape the LLM is instructed to respond with.
type judgeVerdictJSON struct {
	Verdict                 string `json:"verdict"`
	Severity                string `json:"severity"`
	RecommendMedicalConsult bool   `json:"recommend_medical_consult"`
	Reasoning               string `json:"reasoning"`
	BriefReasoning          string `json:"brief_reasoning"`
	CitedEvidence           []int  `json:"cited_evidence"`
}

// Judge implements data.ClaimJudge.
func (c *JudgeClient) Judge(
	ctx context.Context,
	req *data.JudgeRequest,
) (*data.JudgeVerdict, error) {
	payload := extractionCompletionRequest{
		Model: c.model,
		Messages: []extractionChatMessage{
			{Role: judgeChatRoleUser, Content: buildJudgePrompt(req)},
		},
		MaxTokens: judgeMaxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal judge request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create judge request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call judge endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read judge response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"judge endpoint returned error status: %d. Body: %s",
			resp.StatusCode,
			string(respBytes),
		)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal judge response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("judge response contained no choices: %s", respBytes)
	}

	content := stripJSONCodeFence(parsed.Choices[0].Message.Content)
	var raw judgeVerdictJSON
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal judge verdict JSON: %w: %s", err, content)
	}

	verdict, err := model.ParseVerdict(raw.Verdict)
	if err != nil {
		log.Printf(
			"[JudgeClient] unrecognized verdict %q — defaulting to insufficient_evidence",
			raw.Verdict,
		)
		verdict = model.VerdictInsufficientEvidence
	}

	severity, err := model.ParseSeverity(raw.Severity)
	recommendConsult := raw.RecommendMedicalConsult
	if err != nil {
		log.Printf(
			"[JudgeClient] unrecognized severity %q — defaulting to serious and forcing medical consult recommendation",
			raw.Severity,
		)
		severity = model.SeveritySerious
		recommendConsult = true
	}

	// Validate citations against the actual evidence list — an out-of-range
	// index is a hallucinated citation, not a real one.
	validCitations := make([]int, 0, len(raw.CitedEvidence))
	for _, idx := range raw.CitedEvidence {
		if idx >= 0 && idx < len(req.Evidence) {
			validCitations = append(validCitations, idx)
		} else {
			log.Printf(
				"[JudgeClient] dropping out-of-range citation index %d (evidence has %d items)",
				idx,
				len(req.Evidence),
			)
		}
	}

	// Structural safeguard, not just a prompt instruction: any verdict other
	// than InsufficientEvidence requires at least one valid citation. Without
	// one, the model's confidence isn't trustworthy — force the honest
	// fallback rather than surface an ungrounded verdict.
	if len(validCitations) == 0 && verdict != model.VerdictInsufficientEvidence {
		log.Printf(
			"[JudgeClient] verdict %q had no valid citations — forcing insufficient_evidence for claim %q",
			verdict,
			req.Claim,
		)
		verdict = model.VerdictInsufficientEvidence
	}

	return &data.JudgeVerdict{
		Verdict:                 verdict,
		Severity:                severity,
		RecommendMedicalConsult: recommendConsult,
		Reasoning:               raw.Reasoning,
		BriefReasoning:          raw.BriefReasoning,
		CitedEvidence:           validCitations,
	}, nil
}

// buildJudgePrompt builds the verdict instruction, enumerating req.Evidence
// with the indices CitedEvidence must reference.
func buildJudgePrompt(req *data.JudgeRequest) string {
	var b strings.Builder
	b.WriteString("You are a strict, evidence-only fact-checking judge for a Sexual and ")
	b.WriteString("Reproductive Health (SRH) knowledge base. You must decide whether a claim is ")
	b.WriteString(
		"supported, contradicted, or unaddressed — using ONLY the evidence listed below. ",
	)
	b.WriteString(
		"You have no other context: even if you recognize the topic and believe you know ",
	)
	b.WriteString("the answer from your own training, you must ignore that entirely and judge ")
	b.WriteString(
		"strictly from the evidence provided. If the evidence doesn't address the claim, ",
	)
	b.WriteString("say so — do not fill the gap with outside knowledge.\n\n")

	fmt.Fprintf(&b, "Claim: %q\n\n", req.Claim)

	if len(req.Evidence) == 0 {
		b.WriteString("Evidence: (none retrieved)\n\n")
	} else {
		b.WriteString("Evidence:\n")
		for i, e := range req.Evidence {
			fmt.Fprintf(&b, "[%d] (%s) %s\n", i, e.Source, e.Text)
			if e.TruthTier != "" {
				fmt.Fprintf(&b, "    truth_tier: %s\n", e.TruthTier)
			}
			if e.ChunkText != "" {
				fmt.Fprintf(&b, "    Full source passage: %s\n", e.ChunkText)
			}
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(
		&b,
		"Write the \"reasoning\" field in %s, regardless of what language the evidence "+
			"above is written in — do not translate the evidence itself, just write your "+
			"own reasoning in %s.\n\n",
		languageDisplayNames[req.ResponseLanguage],
		languageDisplayNames[req.ResponseLanguage],
	)

	b.WriteString(
		"Respond with ONLY a JSON object (no markdown, no commentary), with this shape:\n",
	)
	b.WriteString(
		`{"verdict": "...", "severity": "...", "recommend_medical_consult": true/false, ` +
			`"reasoning": "...", "brief_reasoning": "...", "cited_evidence": [0, 2]}` + "\n\n",
	)
	b.WriteString("verdict must be exactly one of: supported, contradicted, partially_supported, ")
	b.WriteString("mixed, insufficient_evidence.\n")
	b.WriteString("- supported: the evidence backs the claim.\n")
	b.WriteString("- contradicted: the evidence directly contradicts the claim.\n")
	b.WriteString("- partially_supported: the evidence is relevant but qualified, conditional, or ")
	b.WriteString("incomplete relative to the claim as stated.\n")
	b.WriteString("- mixed: the evidence genuinely conflicts — some supports, some contradicts.\n")
	b.WriteString("- insufficient_evidence: the evidence doesn't address the claim, or none was ")
	b.WriteString("retrieved. Use this whenever you're not confident the evidence actually covers ")
	b.WriteString("the claim — this never implies the claim itself is right or wrong.\n\n")
	b.WriteString("severity must be exactly one of: routine, serious, emergency — how urgent/")
	b.WriteString(
		"high-stakes the claim's subject matter is if someone acted on wrong information. ",
	)
	b.WriteString(
		"Set recommend_medical_consult to true whenever severity is serious or emergency — ",
	)
	b.WriteString(
		"this system provides quick access to reliable information, it does not replace a ",
	)
	b.WriteString("real doctor, especially for serious or life-threatening situations.\n\n")
	b.WriteString(
		"cited_evidence must list the indices (from the Evidence list above, 0-based) that ",
	)
	b.WriteString(
		"your verdict is actually based on. Required (non-empty) for any verdict other than ",
	)
	b.WriteString("insufficient_evidence — if you cannot point to specific evidence, use ")
	b.WriteString("insufficient_evidence instead.\n\n")
	b.WriteString(
		"Whenever your reasoning references a specific piece of evidence, cite it inline as ",
	)
	b.WriteString(
		`"[N]" using that same 0-based index from the Evidence list — e.g. "evidence [2] ` +
			`states...". Every index you cite inline in reasoning must also appear in cited_evidence, ` +
			"and vice versa: the two must match exactly, so a reader can look up any \"[N]\" in your " +
			"reasoning against the same-numbered item in cited_evidence.\n\n",
	)
	fmt.Fprintf(
		&b,
		"brief_reasoning must be a single, self-contained plain-language sentence in %s "+
			"paraphrasing your reasoning — it will be shown on its own, without the evidence list, "+
			"so it must NEVER use inline \"[N]\" citation markers or otherwise assume the reader can "+
			"see the evidence.\n",
		languageDisplayNames[req.ResponseLanguage],
	)

	return b.String()
}
