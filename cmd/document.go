package cmd

import "github.com/spf13/cobra"

// documentCmd is the parent command for document-related CLI operations.
var documentCmd = &cobra.Command{
	Use:   "document",
	Short: "Manage documents",
}
