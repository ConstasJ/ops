package compose

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ModifyResult contains details of the YAML modification operation
type ModifyResult struct {
	OriginalImage string
	NewImage      string
	BackupPath    string
	ModifiedFile  string
}

// YamlModifier provides safe, indentation-preserving modification of Compose YAML files
type YamlModifier struct{}

// NewYamlModifier returns a new instance of YamlModifier
func NewYamlModifier() *YamlModifier {
	return &YamlModifier{}
}

var (
	servicesHeaderRegex = regexp.MustCompile(`^services:\s*(#.*)?$`)
	serviceNameRegex    = regexp.MustCompile(`^(\s*)(["']?[\w\-.]+["']?):\s*(#.*)?$`)
	imageLineRegex      = regexp.MustCompile(`^(\s*image:\s*)(["']?)([^"'\s#]+)(["']?)(\s*#.*)?$`)
	blockScalarRegex    = regexp.MustCompile(`^(\s*image:\s*)([>|][-+]?)(\s*#.*)?$`)
)

// UpdateImageTag updates the image of a specific service in composeFile and creates a backup
func (m *YamlModifier) UpdateImageTag(composeFile, serviceName, newImage string) (*ModifyResult, error) {
	fileInfo, err := os.Stat(composeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat compose file %s: %w", composeFile, err)
	}

	content, err := os.ReadFile(composeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file %s: %w", composeFile, err)
	}

	// Detect line ending (\r\n or \n)
	lineEnding := "\n"
	if bytes.Contains(content, []byte("\r\n")) {
		lineEnding = "\r\n"
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan compose file lines: %w", err)
	}

	inServices := false
	servicesIndent := -1
	targetServiceIndent := -1
	inTargetService := false

	var originalImage string
	found := false

	cleanServiceName := strings.Trim(serviceName, "\"'")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))

		// Top-level check
		if indent == 0 {
			if servicesHeaderRegex.MatchString(line) {
				inServices = true
				servicesIndent = 0
				targetServiceIndent = -1
				inTargetService = false
				continue
			} else if inServices {
				// Exited services block
				inServices = false
				inTargetService = false
			}
		}

		if inServices {
			if inTargetService {
				// If indent is <= targetServiceIndent, we exited the target service
				if indent <= targetServiceIndent {
					inTargetService = false
				} else {
					// Check if this line is a block scalar image declaration (e.g., image: >- or image: |)
					matchesBlock := blockScalarRegex.FindStringSubmatch(line)
					if len(matchesBlock) > 0 {
						prefix := matchesBlock[1]
						originalImage = matchesBlock[2]
						comment := matchesBlock[3]

						lines[i] = fmt.Sprintf("%s%s%s", prefix, newImage, comment)

						// Delete all following lines that are part of this block scalar (indented deeper than image line)
						imageIndent := indent
						deleteCount := 0
						for j := i + 1; j < len(lines); j++ {
							nextLine := lines[j]
							trimmedNext := strings.TrimSpace(nextLine)
							if trimmedNext == "" {
								deleteCount++
								continue
							}
							nextIndent := len(nextLine) - len(strings.TrimLeft(nextLine, " "))
							if nextIndent > imageIndent {
								deleteCount++
							} else {
								break
							}
						}
						if deleteCount > 0 {
							lines = append(lines[:i+1], lines[i+1+deleteCount:]...)
						}

						found = true
						inTargetService = false
						break
					}

					// Check for single-line image
					matches := imageLineRegex.FindStringSubmatch(line)
					if len(matches) > 0 {
						// matches: [0: full, 1: prefix, 2: openQuote, 3: image, 4: closeQuote, 5: comment]
						prefix := matches[1]
						openQuote := matches[2]
						origImg := matches[3]
						closeQuote := matches[4]
						comment := matches[5]

						originalImage = origImg
						// Format new image preserving quotes if present
						quote := ""
						if openQuote != "" || closeQuote != "" {
							quote = `"`
							if openQuote == "'" || closeQuote == "'" {
								quote = `'`
							}
						}

						lines[i] = fmt.Sprintf("%s%s%s%s%s", prefix, quote, newImage, quote, comment)
						found = true
						inTargetService = false
						break
					}
				}
			}

			if !inTargetService && !found {
				// Check if this line defines a service
				matches := serviceNameRegex.FindStringSubmatch(line)
				if len(matches) > 0 {
					svcIndent := len(matches[1])
					// Must be indented under services (indent > servicesIndent)
					if svcIndent > servicesIndent {
						svcName := strings.Trim(matches[2], "\"'")
						if svcName == cleanServiceName {
							inTargetService = true
							targetServiceIndent = svcIndent
						}
					}
				}
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("service %q with an image field not found in %s", serviceName, composeFile)
	}

	// Join modified lines
	modifiedContent := strings.Join(lines, lineEnding)
	if strings.HasSuffix(string(content), lineEnding) {
		modifiedContent += lineEnding
	}

	// Validate modified content is valid YAML
	var checkNode yaml.Node
	if err := yaml.Unmarshal([]byte(modifiedContent), &checkNode); err != nil {
		return nil, fmt.Errorf("modified compose YAML failed syntax validation: %w", err)
	}

	// Create backup
	timestamp := time.Now().Format("20060102150405.000000000")
	backupPath := fmt.Sprintf("%s.ops-bak-%s", composeFile, timestamp)
	if err := os.WriteFile(backupPath, content, fileInfo.Mode()); err != nil {
		return nil, fmt.Errorf("failed to create backup file %s: %w", backupPath, err)
	}
	_ = os.Chtimes(backupPath, fileInfo.ModTime(), fileInfo.ModTime())

	// Write modified file atomically
	tmpFile := fmt.Sprintf("%s.ops-tmp-%s", composeFile, timestamp)
	if err := os.WriteFile(tmpFile, []byte(modifiedContent), fileInfo.Mode()); err != nil {
		_ = os.Remove(backupPath)
		return nil, fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tmpFile, composeFile); err != nil {
		_ = os.Remove(tmpFile)
		_ = os.Remove(backupPath)
		return nil, fmt.Errorf("failed to atomically update compose file: %w", err)
	}

	return &ModifyResult{
		OriginalImage: originalImage,
		NewImage:      newImage,
		BackupPath:    backupPath,
		ModifiedFile:  composeFile,
	}, nil
}

// RestoreBackup restores a previously created backup over the compose file
func (m *YamlModifier) RestoreBackup(backupPath, composeFile string) error {
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("failed to stat backup file %s: %w", backupPath, err)
	}

	content, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file %s: %w", backupPath, err)
	}

	tmpFile := fmt.Sprintf("%s.restore-tmp-%d", composeFile, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, content, backupInfo.Mode()); err != nil {
		return fmt.Errorf("failed to write temporary restore file: %w", err)
	}

	if err := os.Rename(tmpFile, composeFile); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to atomic rename restore file: %w", err)
	}

	_ = os.Chtimes(composeFile, backupInfo.ModTime(), backupInfo.ModTime())
	return nil
}
