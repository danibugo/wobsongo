package cmd

import "github.com/spf13/cobra"

// doclingCmd is the parent command for Docling Serve testing utilities.
var doclingCmd = &cobra.Command{
	Use:   "docling",
	Short: "Docling Serve testing utilities",
}
