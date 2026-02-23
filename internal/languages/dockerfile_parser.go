package languages

import (
	"bufio"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&DockerfileParser{})
}

// DockerfileParser implements LanguageParser for Dockerfiles
type DockerfileParser struct{}

func (p *DockerfileParser) Name() string {
	return "dockerfile"
}

func (p *DockerfileParser) Extensions() []string {
	return []string{".dockerfile"}
}

func (p *DockerfileParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	baseName := strings.ToLower(filepath.Base(path))

	return strings.HasSuffix(pathLower, ".dockerfile") ||
		baseName == "dockerfile" ||
		strings.HasPrefix(baseName, "dockerfile.")
}

func (p *DockerfileParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/")
}

func (p *DockerfileParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("Dockerfile: " + fileName + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Detect stages for summary
	stages := p.extractStages(content)
	if len(stages) > 0 {
		contentBuilder.WriteString("Stages: " + strings.Join(stages, ", ") + "\n")
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"dockerfile", "docker", "container"}
	if len(stages) > 1 {
		tags = append(tags, "multi-stage")
	}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        "dockerfile",
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "docker",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

// Regex patterns for Dockerfile parsing
var (
	dockerFromRegex       = regexp.MustCompile(`(?i)^FROM\s+(\S+)(?:\s+AS\s+(\S+))?`)
	dockerArgRegex        = regexp.MustCompile(`(?i)^ARG\s+(\w+)(?:=(.*))?`)
	dockerEnvRegex        = regexp.MustCompile(`(?i)^ENV\s+(\w+)(?:=|\s+)(.*)`)
	dockerExposeRegex     = regexp.MustCompile(`(?i)^EXPOSE\s+(.+)`)
	dockerLabelRegex      = regexp.MustCompile(`(?i)^LABEL\s+(.+)`)
	dockerEntrypointRegex = regexp.MustCompile(`(?i)^ENTRYPOINT\s+(.+)`)
	dockerCmdRegex        = regexp.MustCompile(`(?i)^CMD\s+(.+)`)
	dockerWorkdirRegex    = regexp.MustCompile(`(?i)^WORKDIR\s+(.+)`)
	dockerCopyRegex       = regexp.MustCompile(`(?i)^COPY\s+(?:--from=(\S+)\s+)?(.+)`)
	dockerVolumeRegex     = regexp.MustCompile(`(?i)^VOLUME\s+(.+)`)
)

func (p *DockerfileParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	currentStage := ""
	stageCount := 0

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Handle line continuations
		for strings.HasSuffix(trimmed, "\\") && scanner.Scan() {
			trimmed = strings.TrimSuffix(trimmed, "\\") + " " + strings.TrimSpace(scanner.Text())
		}

		// FROM instruction (base image / stage)
		if matches := dockerFromRegex.FindStringSubmatch(trimmed); matches != nil {
			image := matches[1]
			stageName := matches[2]

			stageCount++
			if stageName != "" {
				currentStage = stageName
			} else {
				currentStage = ""
			}

			// Add base image as symbol
			symbols = append(symbols, Symbol{
				Name:       "FROM:" + image,
				Type:       "constant",
				Line:       lineNum,
				Value:      image,
				Exported:   true,
				DocComment: func() string {
					if stageName != "" {
						return "stage: " + stageName
					}
					return ""
				}(),
				Language: "dockerfile",
			})

			// Add stage as section if named
			if stageName != "" {
				symbols = append(symbols, Symbol{
					Name:       stageName,
					Type:       "section",
					Line:       lineNum,
					Exported:   true,
					DocComment: "FROM " + image,
					Language:   "dockerfile",
				})
			}
			continue
		}

		// ARG instruction
		if matches := dockerArgRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			value := ""
			if len(matches) > 2 {
				value = strings.TrimSpace(matches[2])
			}

			symbols = append(symbols, Symbol{
				Name:     name,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Parent:   currentStage,
				Exported: true,
				Language: "dockerfile",
			})
			continue
		}

		// ENV instruction
		if matches := dockerEnvRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			value := strings.TrimSpace(matches[2])
			// Strip quotes
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
			}

			symbols = append(symbols, Symbol{
				Name:     name,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Parent:   currentStage,
				Exported: true,
				Language: "dockerfile",
			})
			continue
		}

		// EXPOSE instruction
		if matches := dockerExposeRegex.FindStringSubmatch(trimmed); matches != nil {
			ports := strings.Fields(matches[1])
			for _, port := range ports {
				symbols = append(symbols, Symbol{
					Name:     "EXPOSE:" + port,
					Type:     "constant",
					Line:     lineNum,
					Value:    port,
					Parent:   currentStage,
					Exported: true,
					Language: "dockerfile",
				})
			}
			continue
		}

		// ENTRYPOINT instruction
		if matches := dockerEntrypointRegex.FindStringSubmatch(trimmed); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     "ENTRYPOINT",
				Type:     "function",
				Line:     lineNum,
				Value:    matches[1],
				Parent:   currentStage,
				Exported: true,
				Language: "dockerfile",
			})
			continue
		}

		// CMD instruction
		if matches := dockerCmdRegex.FindStringSubmatch(trimmed); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     "CMD",
				Type:     "function",
				Line:     lineNum,
				Value:    matches[1],
				Parent:   currentStage,
				Exported: true,
				Language: "dockerfile",
			})
			continue
		}

		// WORKDIR instruction
		if matches := dockerWorkdirRegex.FindStringSubmatch(trimmed); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     "WORKDIR",
				Type:     "constant",
				Line:     lineNum,
				Value:    matches[1],
				Parent:   currentStage,
				Exported: true,
				Language: "dockerfile",
			})
			continue
		}

		// VOLUME instruction
		if matches := dockerVolumeRegex.FindStringSubmatch(trimmed); matches != nil {
			volumeSpec := strings.TrimSpace(matches[1])
			// Handle JSON array format: ["vol1", "vol2"]
			if strings.HasPrefix(volumeSpec, "[") {
				volumeSpec = strings.Trim(volumeSpec, "[]")
				volumes := strings.Split(volumeSpec, ",")
				for _, vol := range volumes {
					vol = strings.TrimSpace(vol)
					vol = strings.Trim(vol, `"'`)
					symbols = append(symbols, Symbol{
						Name:     "VOLUME:" + vol,
						Type:     "constant",
						Line:     lineNum,
						Value:    vol,
						Parent:   currentStage,
						Exported: true,
						Language: "dockerfile",
					})
				}
			} else {
				// Space-separated format
				volumes := strings.Fields(volumeSpec)
				for _, vol := range volumes {
					symbols = append(symbols, Symbol{
						Name:     "VOLUME:" + vol,
						Type:     "constant",
						Line:     lineNum,
						Value:    vol,
						Parent:   currentStage,
						Exported: true,
						Language: "dockerfile",
					})
				}
			}
			continue
		}
	}

	return symbols
}

func (p *DockerfileParser) extractStages(content string) []string {
	var stages []string
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := dockerFromRegex.FindStringSubmatch(line); matches != nil {
			if matches[2] != "" {
				stages = append(stages, matches[2])
			}
		}
	}

	return stages
}
