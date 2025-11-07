package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rmitchellscott/rm-qmd-verify/internal/qmldiff"
)

func main() {
	qmdPath := flag.String("qmd", "", "Path to QMD file")
	hashtabPath := flag.String("hashtab", "", "Path to hashtab file")
	treePath := flag.String("tree", "", "Path to QML tree directory")
	workers := flag.Int("workers", 4, "Number of worker goroutines")

	flag.Parse()

	if *qmdPath == "" || *hashtabPath == "" || *treePath == "" {
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("Testing tree validation:\n")
	fmt.Printf("  QMD: %s\n", *qmdPath)
	fmt.Printf("  Hashtab: %s\n", *hashtabPath)
	fmt.Printf("  Tree: %s\n", *treePath)
	fmt.Printf("  Workers: %d\n", *workers)
	fmt.Println()

	validator := qmldiff.NewTreeValidator(*qmdPath, *hashtabPath, *treePath)
	result, err := validator.Validate()
	if err != nil {
		log.Fatalf("Validation failed: %v", err)
	}

	fmt.Printf("Validation complete!\n")
	fmt.Printf("  Files processed: %d\n", result.FilesProcessed)
	fmt.Printf("  Files modified: %d\n", result.FilesModified)
	fmt.Printf("  Files with errors: %d\n", result.FilesWithErrors)
	fmt.Printf("  Has hash errors: %v\n", result.HasHashErrors)

	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors encountered:\n")
		for _, err := range result.Errors {
			fmt.Printf("  %s: %s\n", err.FilePath, err.Error)
		}
	}

	if result.FilesWithErrors > 0 || result.HasHashErrors {
		os.Exit(1)
	}
}
