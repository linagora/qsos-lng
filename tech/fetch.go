package tech

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type sonarMeasuresResponse struct {
	Component struct {
		Measures []struct {
			Metric string
			Value  string
		}
	}
}

type sonarIssues struct {
	Total int64
}

// Fetch retrieves all tech-related data from SonarQube
func Fetch(owner, repo, sonarqubeURL, sonarToken string) (*TechData, error) {
	u, err := url.Parse(sonarqubeURL)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse SONARQUBE_URL: %w", err)
	}

	if err := runSonarScannerCLI(owner, repo, u, sonarToken); err != nil {
		return nil, err
	}

	// XXX Sonarqube takes some time to build the measures after the scanner
	// has sent its result...
	for i := 0; i < 100; i++ {
		data, err := getSonarStats(owner, repo, u, sonarToken)
		if err != nil {
			return nil, err
		}
		if data.LinesOfCode > 0 && data.BrainOverload > 0 {
			if data.Functions == 0 {
				return nil, fmt.Errorf("sonarqube analysis has failed, no function detected")
			}
			return data, nil
		}
		log.Printf("measures not yet available in Sonarqube")
		time.Sleep(1 * time.Second)
	}
	return getSonarStats(owner, repo, u, sonarToken)
}

func runSonarScannerCLI(owner, repo string, sonarqubeURL *url.URL, sonarToken string) error {
	component := owner + "-" + repo
	tmpDir, err := os.MkdirTemp("", component+"-")
	if err != nil {
		return fmt.Errorf("Cannot create a temporary dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	cmd := exec.Command("git", "clone", "--depth=1",
		fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), ".")
	cmd.Dir = tmpDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Cannot clone git repository: %w", err)
	}

	// Create temp directory for sonar-scanner (will be mounted as /tmp in container)
	containerTmpDir, err := os.MkdirTemp("", component+"-tmp-")
	if err != nil {
		return fmt.Errorf("Cannot create temp dir: %w", err)
	}
	defer os.RemoveAll(containerTmpDir)

	// TODO make the command configurable
	cmd = exec.Command(
		"docker", "run", "--rm", "--net=host",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-e", fmt.Sprintf(`SONAR_HOST_URL=%s`, sonarqubeURL),
		"-e", fmt.Sprintf(`SONAR_TOKEN=%s`, sonarToken),
		"-e", "SONAR_USER_HOME=/tmp",
		"-v", fmt.Sprintf(`%s:/usr/src`, tmpDir),
		"-v", fmt.Sprintf(`%s:/tmp`, containerTmpDir),
		"sonarsource/sonar-scanner-cli",
		fmt.Sprintf(`-Dsonar.projectKey=%s`, component),
		"-Dsonar.sources=.",
	)
	cmd.Dir = tmpDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Cannot run sonar-scanner-cli: %w", err)
	}
	return nil
}

func getSonarStats(owner, repo string, sonarqubeURL *url.URL, sonarToken string) (*TechData, error) {
	component := owner + "-" + repo
	data, err := getSonarMeasures(component, sonarqubeURL, sonarToken)
	if err != nil {
		return nil, fmt.Errorf("cannot get sonar stats: %w", err)
	}
	if data.LinesOfCode == 0 {
		return data, nil
	}
	nb, err := getSonarBrainOverloadIssues(component, sonarqubeURL, sonarToken)
	if err != nil {
		return nil, fmt.Errorf("cannot get sonar issues: %w", err)
	}
	data.BrainOverload = nb

	log.Printf("\n--- Sonarqube Statistics ---\n")
	log.Printf("Number of lines of code: %d\n", data.LinesOfCode)
	log.Printf("Number of functions:     %d\n", data.Functions)
	log.Printf("Cyclomatic complexity:   %d\n", data.CyclomaticComplexity)
	log.Printf("Cognitive complexity:    %d\n", data.CognitiveComplexity)
	log.Printf("Brain-overload issues:   %d\n", data.BrainOverload)
	log.Printf("Number of code smells:   %d\n", data.CodeSmells)
	log.Printf("Duplication density:     %.1f\n", data.DuplicationDensity)

	return data, nil
}

func getSonarMeasures(component string, sonarqubeURL *url.URL, sonarToken string) (*TechData, error) {
	cloned := *sonarqubeURL
	cloned.Path = "/api/measures/component"
	cloned.RawQuery = url.Values{
		"component":  []string{component},
		"metricKeys": []string{"ncloc,functions,code_smells,complexity,cognitive_complexity,duplicated_lines_density"},
	}.Encode()
	req, err := http.NewRequest(http.MethodGet, cloned.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Cannot create request: %w", err)
	}
	req.SetBasicAuth(sonarToken, "")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error on request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response: %d", res.StatusCode)
	}
	defer res.Body.Close()

	data := &TechData{}
	var response sonarMeasuresResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	for _, measure := range response.Component.Measures {
		switch measure.Metric {
		case "ncloc":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid ncloc value: %w", err)
			}
			data.LinesOfCode = nb
		case "functions":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid functions value: %w", err)
			}
			data.Functions = nb
		case "code_smells":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid code_smells value: %w", err)
			}
			data.CodeSmells = nb
		case "complexity":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid complexity value: %w", err)
			}
			data.CyclomaticComplexity = nb
		case "cognitive_complexity":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid cognitive_complexity value: %w", err)
			}
			data.CognitiveComplexity = nb
		case "duplicated_lines_density":
			nb, err := strconv.ParseFloat(measure.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid duplicated_lines_density value: %w", err)
			}
			data.DuplicationDensity = nb
		}
	}

	return data, nil
}

func getSonarBrainOverloadIssues(component string, sonarqubeURL *url.URL, sonarToken string) (int64, error) {
	cloned := *sonarqubeURL
	cloned.Path = "/api/issues/search"
	cloned.RawQuery = url.Values{
		"components": []string{component},
		"tags":       []string{"brain-overload"},
	}.Encode()
	req, err := http.NewRequest(http.MethodGet, cloned.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("Cannot create request: %w", err)
	}
	req.SetBasicAuth(sonarToken, "")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Error on request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response: %d", res.StatusCode)
	}
	defer res.Body.Close()

	var data sonarIssues
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return 0, fmt.Errorf("invalid response: %w", err)
	}
	return data.Total, nil
}
