package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Commit represents a git commit
type Commit struct {
	Hash      string
	Author    string
	Date      time.Time
	Subject   string
	Body      string
	Files     []string
	Additions int
	Deletions int
}

// Report contains all the data for the HTML report
type Report struct {
	CompanyName string
	LogoPath    string
	StartDate   time.Time
	EndDate     time.Time
	RepoPath    string
	Commits     []Commit
	GeneratedAt time.Time
}

func main() {
	// Parse command line arguments
	startDateStr := flag.String("start", "", "Start date (YYYY-MM-DD)")
	endDateStr := flag.String("end", "", "End date (YYYY-MM-DD)")
	repoPath := flag.String("repo", ".", "Path to the git repository")
	logoPath := flag.String("logo", "", "Path to company logo")
	companyName := flag.String("company", "Company", "Name of the company")
	outputFile := flag.String("output", "", "Output HTML file path (default: YYYY-MM-DD-COMPANY-report.html)")

	flag.Parse()

	// Validate required arguments
	if *startDateStr == "" || *endDateStr == "" {
		fmt.Println("Error: Start and end dates are required")
		flag.Usage()
		os.Exit(1)
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", *startDateStr)
	if err != nil {
		log.Fatalf("Error parsing start date: %v", err)
	}

	endDate, err := time.Parse("2006-01-02", *endDateStr)
	if err != nil {
		log.Fatalf("Error parsing end date: %v", err)
	}
	
	// Add one day to end date to make it inclusive
	endDate = endDate.AddDate(0, 0, 1)

	// Verify repository exists
	absRepoPath, err := filepath.Abs(*repoPath)
	if err != nil {
		log.Fatalf("Error resolving repository path: %v", err)
	}

	if _, err := os.Stat(filepath.Join(absRepoPath, ".git")); os.IsNotExist(err) {
		log.Fatalf("Error: %s is not a git repository", absRepoPath)
	}

	// Get git commits
	commits, err := getCommits(absRepoPath, startDate, endDate)
	if err != nil {
		log.Fatalf("Error retrieving commits: %v", err)
	}

	// Create the report
	report := Report{
		CompanyName: *companyName,
		LogoPath:    *logoPath,
		StartDate:   startDate,
		EndDate:     endDate.AddDate(0, 0, -1), // Subtract a day for display
		RepoPath:    absRepoPath,
		Commits:     commits,
		GeneratedAt: time.Now(),
	}

	// Determine output filename
	outputFileName := *outputFile
	if outputFileName == "" {
		// Create filename in format YYYY-MM-DD-COMPANY-report.html
		sanitizedCompanyName := strings.ReplaceAll(*companyName, " ", "-")
		sanitizedCompanyName = strings.ReplaceAll(sanitizedCompanyName, "/", "-")
		outputFileName = fmt.Sprintf("%s-%s-report.html", 
			endDate.AddDate(0, 0, -1).Format("2006-01-02"), 
			sanitizedCompanyName)
	}

	// Generate HTML report
	err = generateHTMLReport(report, outputFileName)
	if err != nil {
		log.Fatalf("Error generating HTML report: %v", err)
	}

	fmt.Printf("Report generated successfully: %s\n", outputFileName)
}

func getCommits(repoPath string, startDate, endDate time.Time) ([]Commit, error) {
	// Format dates for git log
	startDateStr := startDate.Format("2006-01-02")
	endDateStr := endDate.Format("2006-01-02")

	// Run git log command
	cmd := exec.Command("git", "log", "--since="+startDateStr, "--until="+endDateStr, "--format=%H%n%an%n%ad%n%s%n%b%n<END_COMMIT>")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log command failed: %v", err)
	}

	// Parse commits
	commits := []Commit{}
	commitTexts := strings.Split(string(output), "<END_COMMIT>\n")
	
	for _, commitText := range commitTexts {
		commitText = strings.TrimSpace(commitText)
		if commitText == "" {
			continue
		}

		lines := strings.Split(commitText, "\n")
		if len(lines) < 4 {
			continue
		}

		hash := lines[0]
		author := lines[1]
		dateStr := lines[2]
		subject := lines[3]
		
		// Parse body
		body := ""
		if len(lines) > 4 {
			body = strings.Join(lines[4:], "\n")
			body = strings.TrimSpace(body)
		}

		// Parse date
		date, err := time.Parse("Mon Jan 2 15:04:05 2006 -0700", dateStr)
		if err != nil {
			// Try alternative format
			date, err = time.Parse("Mon Jan 2 15:04:05 2006", dateStr)
			if err != nil {
				fmt.Printf("Warning: Could not parse date '%s' for commit %s\n", dateStr, hash)
				continue
			}
		}

		// Get changed files
		files, additions, deletions, err := getCommitChanges(repoPath, hash)
		if err != nil {
			fmt.Printf("Warning: Could not get changes for commit %s: %v\n", hash, err)
		}

		commits = append(commits, Commit{
			Hash:      hash,
			Author:    author,
			Date:      date,
			Subject:   subject,
			Body:      body,
			Files:     files,
			Additions: additions,
			Deletions: deletions,
		})
	}

	return commits, nil
}

func getCommitChanges(repoPath, hash string) ([]string, int, int, error) {
	// Get files changed
	cmd := exec.Command("git", "show", "--name-only", "--format=", hash)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, 0, 0, err
	}

	files := []string{}
	for _, file := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if file != "" {
			files = append(files, file)
		}
	}

	// Get stats
	cmd = exec.Command("git", "show", "--stat", "--format=", hash)
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err != nil {
		return files, 0, 0, err
	}

	stats := string(output)
	additions := 0
	deletions := 0

	// Parse last line for +/- stats
	lines := strings.Split(stats, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.Contains(line, "insertion") || strings.Contains(line, "deletion") {
			parts := strings.Split(line, ", ")
			for _, part := range parts {
				if strings.Contains(part, "insertion") {
					fmt.Sscanf(part, "%d insertion", &additions)
				} else if strings.Contains(part, "deletion") {
					fmt.Sscanf(part, "%d deletion", &deletions)
				}
			}
			break
		}
	}

	return files, additions, deletions, nil
}

func generateHTMLReport(report Report, outputFile string) error {
	// Create a new template with additional functions
	funcMap := template.FuncMap{
		"slice": func(s string, i, j int) string {
			if i >= len(s) {
				return ""
			}
			if j > len(s) {
				j = len(s)
			}
			return s[i:j]
		},
	}
	
	// Parse template from file
	tmpl, err := template.New("report.html").Funcs(funcMap).ParseFiles("templates/report.html")
	if err != nil {
		// If template file doesn't exist, create the directory and file first
		if os.IsNotExist(err) {
			err = os.MkdirAll("templates", 0755)
			if err != nil {
				return fmt.Errorf("error creating templates directory: %v", err)
			}
			
			// Check if the template file exists in the current directory
			if _, err := os.Stat("templates/report.html"); os.IsNotExist(err) {
				fmt.Println("Template file not found. Please create 'templates/report.html' before running.")
				os.Exit(1)
			}
			
			// Try again after creating the directory
			tmpl, err = template.New("report.html").Funcs(funcMap).ParseFiles("templates/report.html")
			if err != nil {
				return fmt.Errorf("error parsing template after creating directory: %v", err)
			}
		} else {
			return fmt.Errorf("error parsing template: %v", err)
		}
	}

	// Create output file
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Execute template
	err = tmpl.Execute(file, report)
	if err != nil {
		return err
	}

	return nil
}