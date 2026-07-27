package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kairosedubf/wobsongo/external"
	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/spf13/cobra"
)

// judgeEvalFile is the top-level shape of the fixture file — analyzer and
// judge fixtures are tested independently, mirroring how ClaimAnalyzerClient
// and JudgeClient are each called directly (not through a live retrieval
// pass against a real DB) — same isolation-testing philosophy as
// eval_extraction.go's ExtractionClient fixtures.
type judgeEvalFile struct {
	AnalyzerFixtures []analyzerEvalFixture `json:"analyzer_fixtures"`
	JudgeFixtures    []judgeEvalFixture    `json:"judge_fixtures"`
}

// analyzerEvalFixture is a single golden-set case for the scope/decomposition analyzer.
type analyzerEvalFixture struct {
	Label                string `json:"label"`
	Message              string `json:"message"`
	ExpectedInScope      bool   `json:"expected_in_scope"`
	ExpectedMinSubClaims int    `json:"expected_min_sub_claims"`
}

// judgeEvalFixture is a single golden-set case for the verdict judge — the
// evidence is hand-authored directly in the fixture (not retrieved), so this
// eval is deterministic with respect to what's in any particular DB.
type judgeEvalFixture struct {
	Label                           string                   `json:"label"`
	Claim                           string                   `json:"claim"`
	Evidence                        []judgeEvalEvidenceInput `json:"evidence"`
	ExpectedVerdict                 string                   `json:"expected_verdict"`
	ExpectedSeverity                string                   `json:"expected_severity,omitempty"`
	ExpectedRecommendMedicalConsult *bool                    `json:"expected_recommend_medical_consult,omitempty"`
}

type judgeEvalEvidenceInput struct {
	Source    string `json:"source"`
	Text      string `json:"text"`
	ChunkText string `json:"chunk_text"`
	TruthTier string `json:"truth_tier"`
}

var evalJudgeCmd = &cobra.Command{
	Use:   "eval-judge [fixture-file]",
	Short: "Check the claim-analyzer and judge prompts against a golden set of fixtures",
	Long: "Calls the real claim-analyzer and judge endpoints for each fixture in the given\n" +
		"JSON file and reports whether the response matches expectations. Judge fixtures\n" +
		"carry hand-authored evidence directly (no live retrieval against a real DB), so\n" +
		"results are deterministic with respect to any particular document set. This is a\n" +
		"prompt-quality check against a live LLM, not a deterministic unit test — it costs\n" +
		"real tokens and isn't wired into go test/CI. Read-only: no --apply flag, nothing\n" +
		"is mutated.",
	Args: cobra.ExactArgs(1),
	Run:  runEvalJudge,
}

func runEvalJudge(cmd *cobra.Command, args []string) {
	fixturePath := args[0]
	cfg := appconfig.NewConfig(EnvFile)

	if err := appconfig.IsClaimCheckOK(cfg.ClaimCheckConfig); err != nil {
		cmd.PrintErrf("Config error: %s\n", err.Error())
		os.Exit(1)
		return
	}

	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		cmd.PrintErrf("Failed to read fixture file: %s\n", err.Error())
		os.Exit(1)
		return
	}

	var fixtures judgeEvalFile
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		cmd.PrintErrf("Failed to parse fixture file: %s\n", err.Error())
		os.Exit(1)
		return
	}

	analyzerClient := external.NewClaimAnalyzerClient(
		cfg.ClaimCheckConfig.BaseURL,
		cfg.ClaimCheckConfig.Model,
		cfg.ClaimCheckConfig.APIKey,
	)
	judgeClient := external.NewJudgeClient(
		cfg.ClaimCheckConfig.BaseURL,
		cfg.ClaimCheckConfig.Model,
		cfg.ClaimCheckConfig.APIKey,
	)

	ctx := cmd.Context()
	var passed, failed int

	for _, fixture := range fixtures.AnalyzerFixtures {
		if ok, detail := evalAnalyzerFixture(ctx, analyzerClient, fixture); ok {
			passed++
			cmd.Printf("PASS %s\n", fixture.Label)
		} else {
			failed++
			cmd.Printf("FAIL %s: %s\n", fixture.Label, detail)
		}
	}

	for _, fixture := range fixtures.JudgeFixtures {
		if ok, detail := evalJudgeFixture(ctx, judgeClient, fixture); ok {
			passed++
			cmd.Printf("PASS %s\n", fixture.Label)
		} else {
			failed++
			cmd.Printf("FAIL %s: %s\n", fixture.Label, detail)
		}
	}

	cmd.Printf("\n%d/%d passed\n", passed, passed+failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// evalAnalyzerFixture checks a single analyzer fixture, returning whether it
// passed and, if not, a description of the mismatch.
func evalAnalyzerFixture(
	ctx context.Context,
	client *external.ClaimAnalyzerClient,
	fixture analyzerEvalFixture,
) (bool, string) {
	analysis, err := client.Analyze(ctx, fixture.Message)
	if err != nil {
		return false, fmt.Sprintf("analyze call failed: %s", err.Error())
	}
	if analysis.InScope != fixture.ExpectedInScope {
		return false, fmt.Sprintf(
			"expected in_scope=%v, got %v (refusal_reason=%q)",
			fixture.ExpectedInScope, analysis.InScope, analysis.RefusalReason,
		)
	}
	if fixture.ExpectedInScope && len(analysis.SubClaims) < fixture.ExpectedMinSubClaims {
		return false, fmt.Sprintf(
			"expected at least %d sub-claims, got %d: %v",
			fixture.ExpectedMinSubClaims, len(analysis.SubClaims), analysis.SubClaims,
		)
	}
	return true, ""
}

// evalJudgeFixture checks a single judge fixture, returning whether it
// passed and, if not, a description of the mismatch.
func evalJudgeFixture(
	ctx context.Context,
	client *external.JudgeClient,
	fixture judgeEvalFixture,
) (bool, string) {
	evidence := make([]data.JudgeEvidence, len(fixture.Evidence))
	for i, e := range fixture.Evidence {
		evidence[i] = data.JudgeEvidence{
			Source:    e.Source,
			Text:      e.Text,
			ChunkText: e.ChunkText,
			TruthTier: e.TruthTier,
		}
	}

	verdict, err := client.Judge(ctx, &data.JudgeRequest{Claim: fixture.Claim, Evidence: evidence})
	if err != nil {
		return false, fmt.Sprintf("judge call failed: %s", err.Error())
	}

	expectedVerdict, err := model.ParseVerdict(fixture.ExpectedVerdict)
	if err != nil {
		return false, fmt.Sprintf("invalid expected_verdict %q in fixture", fixture.ExpectedVerdict)
	}
	if verdict.Verdict != expectedVerdict {
		return false, fmt.Sprintf(
			"expected verdict %q, got %q (reasoning: %s)",
			expectedVerdict, verdict.Verdict, verdict.Reasoning,
		)
	}

	if fixture.ExpectedSeverity != "" {
		expectedSeverity, err := model.ParseSeverity(fixture.ExpectedSeverity)
		if err != nil {
			return false, fmt.Sprintf(
				"invalid expected_severity %q in fixture",
				fixture.ExpectedSeverity,
			)
		}
		if verdict.Severity != expectedSeverity {
			return false, fmt.Sprintf(
				"expected severity %q, got %q",
				expectedSeverity,
				verdict.Severity,
			)
		}
	}

	if fixture.ExpectedRecommendMedicalConsult != nil &&
		verdict.RecommendMedicalConsult != *fixture.ExpectedRecommendMedicalConsult {
		return false, fmt.Sprintf(
			"expected recommend_medical_consult=%v, got %v",
			*fixture.ExpectedRecommendMedicalConsult, verdict.RecommendMedicalConsult,
		)
	}

	return true, ""
}
