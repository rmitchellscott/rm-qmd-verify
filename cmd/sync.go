package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/rmitchellscott/rm-qmd-verify/internal/logging"
)

var (
	syncRepo   string
	syncBranch string
	syncDir    string
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync hashtables from GitHub",
	Long:  "Download and update hashtables from the GitHub repository. Overwrites existing files but never deletes local files.",
	Run:   runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().StringVar(&syncRepo, "repo", "rmitchellscott/rm-qmd-verify", "GitHub repository (owner/repo)")
	syncCmd.Flags().StringVar(&syncBranch, "branch", "main", "Branch to sync from")
	syncCmd.Flags().StringVar(&syncDir, "dir", "./hashtables", "Destination directory for hashtables")
}

type GitHubTreeNode struct {
	Path string `json:"path"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

type GitHubTree struct {
	Tree []GitHubTreeNode `json:"tree"`
}

func runSync(cmd *cobra.Command, args []string) {
	logging.Info(logging.ComponentStartup, "Syncing hashtables from %s (branch: %s)", syncRepo, syncBranch)
	logging.Info(logging.ComponentStartup, "Destination directory: %s", syncDir)

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", syncRepo, syncBranch)

	resp, err := http.Get(apiURL)
	if err != nil {
		logging.Error(logging.ComponentStartup, "Failed to fetch repository tree: %v", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logging.Error(logging.ComponentStartup, "GitHub API returned status %d", resp.StatusCode)
		os.Exit(1)
	}

	var tree GitHubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		logging.Error(logging.ComponentStartup, "Failed to parse GitHub response: %v", err)
		os.Exit(1)
	}

	var hashtableFiles []GitHubTreeNode
	for _, node := range tree.Tree {
		if node.Type == "blob" && strings.HasPrefix(node.Path, "hashtables/") {
			hashtableFiles = append(hashtableFiles, node)
		}
	}

	if len(hashtableFiles) == 0 {
		logging.Info(logging.ComponentStartup, "No hashtable files found in repository")
		return
	}

	logging.Info(logging.ComponentStartup, "Found %d hashtable files", len(hashtableFiles))

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		logging.Error(logging.ComponentStartup, "Failed to create destination directory: %v", err)
		os.Exit(1)
	}

	downloaded := 0
	for _, file := range hashtableFiles {
		relPath := strings.TrimPrefix(file.Path, "hashtables/")
		destPath := filepath.Join(syncDir, relPath)

		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			logging.Error(logging.ComponentStartup, "Failed to create directory %s: %v", destDir, err)
			continue
		}

		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", syncRepo, syncBranch, file.Path)

		if err := downloadFile(rawURL, destPath); err != nil {
			logging.Error(logging.ComponentStartup, "Failed to download %s: %v", file.Path, err)
			continue
		}

		logging.Info(logging.ComponentStartup, "Downloaded: %s", relPath)
		downloaded++
	}

	logging.Info(logging.ComponentStartup, "Successfully synced %d/%d hashtable files", downloaded, len(hashtableFiles))
}

func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
