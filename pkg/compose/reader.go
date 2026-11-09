package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type rawComposeFile struct {
	Services map[string]struct {
		Image string `yaml:"image"`
	} `yaml:"services"`
}

// GetDeclaredImage inspects Compose configuration files and extracts the declared image for a given service.
// It prioritizes compose-go loader with variable interpolation and falls back to structured YAML parsing if loader fails.
func GetDeclaredImage(composeFiles []string, workingDir string, serviceName string) (string, error) {
	if len(composeFiles) == 0 || serviceName == "" {
		return "", nil
	}

	cleanServiceName := strings.Trim(serviceName, "\"'")
	wDir := workingDir
	if wDir == "" && len(composeFiles) > 0 {
		wDir = filepath.Dir(composeFiles[0])
	}

	// 1. Try compose-go loader first
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
		ctx := context.Background()
		project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
			WorkingDir:  wDir,
			ConfigFiles: existingFiles,
		}, loader.WithSkipValidation, func(opts *loader.Options) {
			opts.SetProjectName("declared-reader", true)
		})

		if err == nil && project != nil {
			if svc, err := project.GetService(cleanServiceName); err == nil && svc.Image != "" {
				if isValidImageRef(svc.Image) {
					return svc.Image, nil
				}
			}
		}
	}

	// 2. Fallback: Parse structured YAML without requiring complete environment interpolation
	// Reverse order so override files take precedence
	var envMap map[string]string
	if wDir != "" {
		envMap, _ = godotenv.Read(filepath.Join(wDir, ".env"))
	}

	for i := len(composeFiles) - 1; i >= 0; i-- {
		f := composeFiles[i]
		if img := extractImageFromYaml(f, cleanServiceName, envMap); img != "" {
			return img, nil
		}
	}

	return "", nil
}

func extractImageFromYaml(path, serviceName string, envMap map[string]string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var doc rawComposeFile
	if err := yaml.Unmarshal(content, &doc); err != nil || doc.Services == nil {
		return ""
	}

	svc, ok := doc.Services[serviceName]
	if !ok || svc.Image == "" {
		return ""
	}

	img := strings.TrimSpace(svc.Image)
	// Try expanding simple environment variables if present
	if strings.Contains(img, "${") {
		img = expandEnv(img, envMap)
	}

	if isValidImageRef(img) {
		return img
	}

	return ""
}

func expandEnv(s string, envMap map[string]string) string {
	return os.Expand(s, func(key string) string {
		// Handle default values like ${VAR:-default}
		cleanKey := key
		defaultVal := ""
		if idx := strings.Index(key, ":-"); idx != -1 {
			cleanKey = key[:idx]
			defaultVal = key[idx+2:]
		} else if idx := strings.Index(key, "-"); idx != -1 {
			cleanKey = key[:idx]
			defaultVal = key[idx+1:]
		}

		if val, exists := envMap[cleanKey]; exists && val != "" {
			return val
		}
		if val := os.Getenv(cleanKey); val != "" {
			return val
		}
		return defaultVal
	})
}

func isValidImageRef(img string) bool {
	if img == "" {
		return false
	}
	if strings.ContainsAny(img, " \t\r\n>|\"'`") {
		return false
	}
	// If it still contains unexpanded variable interpolation (${...}), it is not a concrete image reference
	if strings.Contains(img, "${") {
		return false
	}
	// A valid image reference shouldn't start or end with punctuation like - or : or /
	if strings.HasPrefix(img, "-") || strings.HasPrefix(img, ":") || strings.HasPrefix(img, "/") {
		return false
	}
	if strings.HasSuffix(img, ":") || strings.HasSuffix(img, "/") {
		return false
	}
	return true
}
