package compose

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// GetDeclaredImage inspects Compose configuration files and extracts the declared image for a given service.
// It prioritizes compose-go loader with variable interpolation and falls back to regex scanning if loader fails.
func GetDeclaredImage(composeFiles []string, workingDir string, serviceName string) (string, error) {
	if len(composeFiles) == 0 || serviceName == "" {
		return "", nil
	}

	cleanServiceName := strings.Trim(serviceName, "\"'")

	// 1. Try compose-go loader
	var existingFiles []types.ConfigFile
	for _, f := range composeFiles {
		if content, err := os.ReadFile(f); err == nil {
			existingFiles = append(existingFiles, types.ConfigFile{
				Filename: f,
				Content:  content,
			})
		}
	}

	if len(existingFiles) > 0 {
		wDir := workingDir
		if wDir == "" && len(composeFiles) > 0 {
			wDir = filepath.Dir(composeFiles[0])
		}

		ctx := context.Background()
		project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
			WorkingDir:  wDir,
			ConfigFiles: existingFiles,
		}, loader.WithSkipValidation, func(opts *loader.Options) {
			opts.SetProjectName("declared-reader", true)
		})

		if err == nil && project != nil {
			if svc, err := project.GetService(cleanServiceName); err == nil && svc.Image != "" {
				return svc.Image, nil
			}
		}
	}

	// 2. Fallback: line-by-line regex scanning (reverse order so override takes precedence)
	for i := len(composeFiles) - 1; i >= 0; i-- {
		f := composeFiles[i]
		if img := scanImageFromYamlFile(f, cleanServiceName); img != "" {
			return img, nil
		}
	}

	return "", nil
}

func scanImageFromYamlFile(path, serviceName string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	inServices := false
	servicesIndent := -1
	targetServiceIndent := -1
	inTargetService := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))

		if indent == 0 {
			if servicesHeaderRegex.MatchString(line) {
				inServices = true
				servicesIndent = 0
				targetServiceIndent = -1
				inTargetService = false
				continue
			} else if inServices {
				inServices = false
				inTargetService = false
			}
		}

		if inServices {
			if inTargetService {
				if indent <= targetServiceIndent {
					inTargetService = false
				} else {
					matches := imageLineRegex.FindStringSubmatch(line)
					if len(matches) > 0 {
						return strings.Trim(matches[3], `"'`)
					}
				}
			}

			if !inTargetService {
				matches := serviceNameRegex.FindStringSubmatch(line)
				if len(matches) > 0 {
					svcIndent := len(matches[1])
					if svcIndent > servicesIndent {
						svcName := strings.Trim(matches[2], `"'`)
						if svcName == serviceName {
							inTargetService = true
							targetServiceIndent = svcIndent
						}
					}
				}
			}
		}
	}

	return ""
}
