package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
)

var (
	qmdPath     string
	hashtabPath string
	treePath    string
	workers     int
	outputJSON  bool
)

var validateTreeCmd = &cobra.Command{
	Use:   "validate-tree",
	Short: "Validate QMD file against full QML tree",
	Long: `Validates a QMD file by applying its diffs to a full QML tree.
This provides accurate validation by actually processing the QML files,
unlike hash-only validation which only checks if hashes exist.

The command requires:
  - A QMD file to validate
  - A hashtab file containing hash-to-string mappings
  - A QML tree directory containing the source QML files

Example:
  qmdverify validate-tree \
    --qmd patch.qmd \
    --hashtab ./hashtables/3.22.0.65-rmppm \
    --tree ./qml-trees/3.22.0.65-rmppm \
    --workers 4`,
	RunE: runValidateTree,
}

func init() {
	validateTreeCmd.Flags().StringVarP(&qmdPath, "qmd", "q", "", "Path to QMD file (required)")
	validateTreeCmd.Flags().StringVar(&hashtabPath, "hashtab", "", "Path to hashtab file (required)")
	validateTreeCmd.Flags().StringVarP(&treePath, "tree", "t", "", "Path to QML tree directory (required)")
	validateTreeCmd.Flags().IntVarP(&workers, "workers", "w", 4, "Number of worker goroutines")
	validateTreeCmd.Flags().BoolVar(&outputJSON, "json", false, "Output results in JSON format")

	validateTreeCmd.MarkFlagRequired("qmd")
	validateTreeCmd.MarkFlagRequired("hashtab")
	validateTreeCmd.MarkFlagRequired("tree")

	rootCmd.AddCommand(validateTreeCmd)
}

func runValidateTree(cmd *cobra.Command, args []string) error {
	// Validate input paths
	if _, err := os.Stat(qmdPath); os.IsNotExist(err) {
		return fmt.Errorf("QMD file not found: %s", qmdPath)
	}
	if _, err := os.Stat(hashtabPath); os.IsNotExist(err) {
		return fmt.Errorf("hashtab file not found: %s", hashtabPath)
	}
	if stat, err := os.Stat(treePath); os.IsNotExist(err) {
		return fmt.Errorf("QML tree directory not found: %s", treePath)
	} else if !stat.IsDir() {
		return fmt.Errorf("tree path must be a directory: %s", treePath)
	}

	if workers < 1 {
		return fmt.Errorf("workers must be at least 1")
	}

	// Validate using CLI (workers parameter is ignored)
	qmldiffBinary := os.Getenv("QMLDIFF_BINARY")
	if qmldiffBinary == "" {
		qmldiffBinary = "./qmldiff"
	}

	batchResult, err := qmldiff.ValidateMultipleQMDsWithCLI([]string{qmdPath}, hashtabPath, treePath, qmldiffBinary)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Extract result for this single file
	var result *qmldiff.TreeValidationResult
	if fileErr, hasError := batchResult.Errors[qmdPath]; hasError {
		return fmt.Errorf("validation error: %w", fileErr)
	}
	result = batchResult.Results[qmdPath]
	if result == nil {
		return fmt.Errorf("no validation result returned")
	}

	// Output results
	if outputJSON {
		return outputResultJSON(result)
	}
	return outputResultText(result)
}

func outputResultText(result *qmldiff.TreeValidationResult) error {
	fmt.Println("Tree Validation Results")
	fmt.Println("=======================")
	fmt.Printf("Files processed: %d\n", result.FilesProcessed)
	fmt.Printf("Files modified:  %d\n", result.FilesModified)
	fmt.Printf("Files with errors: %d\n", result.FilesWithErrors)
	fmt.Printf("Hash lookup errors: %v\n", result.HasHashErrors)

	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, err := range result.Errors {
			fmt.Printf("  - %s: %s\n", err.FilePath, err.Error)
		}
	}

	if result.FilesWithErrors > 0 || result.HasHashErrors {
		fmt.Println("\n❌ Validation failed")
		return fmt.Errorf("validation completed with errors")
	}

	fmt.Println("\n✅ Validation successful")
	return nil
}

type validationResultJSON struct {
	FilesProcessed  int                                 `json:"files_processed"`
	FilesModified   int                                 `json:"files_modified"`
	FilesWithErrors int                                 `json:"files_with_errors"`
	HasHashErrors   bool                                `json:"has_hash_errors"`
	Errors          []qmldiff.TreeValidationError       `json:"errors,omitempty"`
	Success         bool                                `json:"success"`
}

func outputResultJSON(result *qmldiff.TreeValidationResult) error {
	output := validationResultJSON{
		FilesProcessed:  result.FilesProcessed,
		FilesModified:   result.FilesModified,
		FilesWithErrors: result.FilesWithErrors,
		HasHashErrors:   result.HasHashErrors,
		Errors:          result.Errors,
		Success:         result.FilesWithErrors == 0 && !result.HasHashErrors,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	if !output.Success {
		return fmt.Errorf("validation completed with errors")
	}

	return nil
}
