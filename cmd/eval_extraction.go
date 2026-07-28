package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/spf13/cobra"
)

// extractionEvalFixture is a single golden-set case: real chunk text plus the
// fact category a correctly-prompted extraction should assign to it.
type extractionEvalFixture struct {
	Label            string `json:"label"`
	Text             string `json:"text"`
	DocumentTitle    string `json:"document_title"`
	PublisherName    string `json:"publisher_name"`
	PublicationYear  int    `json:"publication_year"`
	ExpectedCategory string `json:"expected_category"`
}

var evalExtractionCmd = &cobra.Command{
	Use:   "eval-extraction [fixture-file]",
	Short: "Check the atomic-knowledge extraction prompt against a golden set of real chunk text",
	Long: "Calls the real extraction endpoint for each fixture in the given JSON file and\n" +
		"reports whether any fact it returns carries the fixture's expected fact category.\n" +
		"This is a prompt-quality check against a live LLM, not a deterministic unit test —\n" +
		"it costs real tokens and isn't wired into go test/CI. Read-only: no --apply flag,\n" +
		"nothing is mutated.",
	Args: cobra.ExactArgs(1),
	Run:  runEvalExtraction,
}

func runEvalExtraction(cmd *cobra.Command, args []string) {
	fixturePath := args[0]
	cfg := appconfig.NewConfig(EnvFile)

	if err := appconfig.IsExtractionOK(cfg.ExtractionConfig); err != nil {
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

	var fixtures []extractionEvalFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		cmd.PrintErrf("Failed to parse fixture file: %s\n", err.Error())
		os.Exit(1)
		return
	}

	extractionClient := external.NewExtractionClient(
		cfg.ExtractionConfig.BaseURL,
		cfg.ExtractionConfig.Model,
		cfg.ExtractionConfig.APIKey,
	)

	ctx := cmd.Context()
	var passed, failed int
	for _, fixture := range fixtures {
		facts, err := extractionClient.Extract(ctx, &data.ExtractionRequest{
			Text:            fixture.Text,
			DocumentTitle:   fixture.DocumentTitle,
			PublisherName:   fixture.PublisherName,
			PublicationYear: fixture.PublicationYear,
		})
		if err != nil {
			failed++
			cmd.Printf("FAIL %s: extraction call failed: %s\n", fixture.Label, err.Error())
			continue
		}

		expected, err := model.ParseFactCategory(fixture.ExpectedCategory)
		if err != nil {
			failed++
			cmd.Printf(
				"FAIL %s: invalid expected_category %q in fixture\n",
				fixture.Label,
				fixture.ExpectedCategory,
			)
			continue
		}

		if factWithCategory(facts, expected) {
			passed++
			cmd.Printf("PASS %s\n", fixture.Label)
			continue
		}

		failed++
		cmd.Printf(
			"FAIL %s: expected a fact categorized %q, got %s\n",
			fixture.Label, expected, describeCategories(facts),
		)
	}

	cmd.Printf("\n%d/%d passed\n", passed, len(fixtures))
	if failed > 0 {
		os.Exit(1)
	}
}

// factWithCategory reports whether any fact in facts carries the given category.
func factWithCategory(facts []data.ExtractedFact, category model.FactCategory) bool {
	for i := range facts {
		if facts[i].Category == category {
			return true
		}
	}
	return false
}

// describeCategories summarizes the categories actually returned, for a
// failing fixture's diagnostic output.
func describeCategories(facts []data.ExtractedFact) string {
	if len(facts) == 0 {
		return "no facts extracted"
	}
	categories := make([]string, len(facts))
	for i := range facts {
		categories[i] = facts[i].Category.String()
	}
	return fmt.Sprintf("categories %v", categories)
}
