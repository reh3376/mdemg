package languages

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&XMLParser{})
}

// XMLParser implements LanguageParser for XML files and variants
type XMLParser struct{}

func (p *XMLParser) Name() string {
	return "xml"
}

func (p *XMLParser) Extensions() []string {
	return []string{
		".xml",
		".xsd",      // XML Schema
		".xsl",      // XSLT Stylesheet
		".xslt",     // XSLT Stylesheet
		".wsdl",     // Web Services
		".svg",      // Scalable Vector Graphics
		".xhtml",    // XHTML
		".plist",    // Apple Property List
		".csproj",   // C# Project
		".vbproj",   // VB.NET Project
		".fsproj",   // F# Project
		".vcxproj",  // Visual C++ Project
		".props",    // MSBuild Props
		".targets",  // MSBuild Targets
		".nuspec",   // NuGet Package Spec
		".resx",     // .NET Resources
		".xaml",     // XAML UI
		".config",   // .NET Config (web.config, app.config)
		".manifest", // Application Manifest
	}
}

func (p *XMLParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	for _, ext := range p.Extensions() {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}
	// Also check for pom.xml specifically
	if strings.HasSuffix(pathLower, "pom.xml") {
		return true
	}
	return false
}

func (p *XMLParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/test/") ||
		strings.Contains(pathLower, "/tests/") ||
		strings.Contains(pathLower, "_test.xml")
}

func (p *XMLParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect XML type
	xmlKind := p.detectXMLKind(fileName, content)

	// Try to parse XML to validate
	decoder := xml.NewDecoder(strings.NewReader(content))
	var rootElement string
	var namespaces []string
	isValid := true

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := token.(xml.StartElement); ok {
			if rootElement == "" {
				rootElement = se.Name.Local
			}
			// Extract namespaces from root element
			for _, attr := range se.Attr {
				if attr.Name.Space == "xmlns" || attr.Name.Local == "xmlns" ||
					strings.HasPrefix(attr.Name.Local, "xmlns:") {
					namespaces = append(namespaces, attr.Value)
				}
			}
			break
		}
	}

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("XML file: %s\n", fileName))
	contentBuilder.WriteString(fmt.Sprintf("Type: %s\n", xmlKind))

	if rootElement != "" {
		contentBuilder.WriteString(fmt.Sprintf("Root element: <%s>\n", rootElement))
	}
	if len(namespaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Namespaces: %d\n", len(namespaces)))
	}

	// Extract additional info based on type
	switch xmlKind {
	case "maven-pom":
		p.extractMavenInfo(&contentBuilder, content)
	case "dotnet-project":
		p.extractDotNetInfo(&contentBuilder, content)
	case "nuget-spec":
		p.extractNuGetInfo(&contentBuilder, content)
	case "svg":
		p.extractSVGInfo(&contentBuilder, content)
	case "xsd-schema":
		p.extractSchemaInfo(&contentBuilder, content)
	}

	if !isValid {
		contentBuilder.WriteString("Warning: Invalid XML\n")
	}

	// Include actual content (truncated)
	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"xml", xmlKind}
	tags = append(tags, concerns...)

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content, xmlKind)
	}

	element := CodeElement{
		Name:     fileName,
		Kind:     xmlKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  "config",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	}

	return []CodeElement{element}, nil
}

func (p *XMLParser) detectXMLKind(fileName string, content string) string {
	fileNameLower := strings.ToLower(fileName)

	// Check by filename
	switch {
	case fileNameLower == "pom.xml":
		return "maven-pom"
	case strings.HasSuffix(fileNameLower, ".csproj"):
		return "dotnet-project"
	case strings.HasSuffix(fileNameLower, ".vbproj"):
		return "dotnet-project"
	case strings.HasSuffix(fileNameLower, ".fsproj"):
		return "dotnet-project"
	case strings.HasSuffix(fileNameLower, ".vcxproj"):
		return "cpp-project"
	case strings.HasSuffix(fileNameLower, ".nuspec"):
		return "nuget-spec"
	case strings.HasSuffix(fileNameLower, ".xsd"):
		return "xsd-schema"
	case strings.HasSuffix(fileNameLower, ".xsl") || strings.HasSuffix(fileNameLower, ".xslt"):
		return "xslt-stylesheet"
	case strings.HasSuffix(fileNameLower, ".wsdl"):
		return "wsdl-service"
	case strings.HasSuffix(fileNameLower, ".svg"):
		return "svg"
	case strings.HasSuffix(fileNameLower, ".xhtml"):
		return "xhtml"
	case strings.HasSuffix(fileNameLower, ".plist"):
		return "plist"
	case strings.HasSuffix(fileNameLower, ".xaml"):
		return "xaml-ui"
	case strings.HasSuffix(fileNameLower, ".resx"):
		return "dotnet-resources"
	case fileNameLower == "web.config" || fileNameLower == "app.config":
		return "dotnet-config"
	case strings.HasSuffix(fileNameLower, ".props") || strings.HasSuffix(fileNameLower, ".targets"):
		return "msbuild"
	}

	// Check content for clues
	if strings.Contains(content, "<project") && strings.Contains(content, "maven") {
		return "maven-pom"
	}
	if strings.Contains(content, "<Project") && strings.Contains(content, "Sdk=") {
		return "dotnet-project"
	}
	if strings.Contains(content, "<svg") {
		return "svg"
	}
	if strings.Contains(content, "<xs:schema") || strings.Contains(content, "<xsd:schema") {
		return "xsd-schema"
	}

	return "xml-data"
}

func (p *XMLParser) extractMavenInfo(builder *strings.Builder, content string) {
	// Extract groupId, artifactId, version
	groupID := extractXMLValue(content, "groupId")
	artifactID := extractXMLValue(content, "artifactId")
	version := extractXMLValue(content, "version")

	if groupID != "" {
		builder.WriteString(fmt.Sprintf("GroupId: %s\n", groupID))
	}
	if artifactID != "" {
		builder.WriteString(fmt.Sprintf("ArtifactId: %s\n", artifactID))
	}
	if version != "" {
		builder.WriteString(fmt.Sprintf("Version: %s\n", version))
	}

	// Count dependencies
	depPattern := regexp.MustCompile(`<dependency>`)
	deps := depPattern.FindAllString(content, -1)
	if len(deps) > 0 {
		builder.WriteString(fmt.Sprintf("Dependencies: %d\n", len(deps)))
	}
}

func (p *XMLParser) extractDotNetInfo(builder *strings.Builder, content string) {
	// Extract SDK and target framework
	if sdk := extractXMLAttr(content, "Project", "Sdk"); sdk != "" {
		builder.WriteString(fmt.Sprintf("SDK: %s\n", sdk))
	}
	if tf := extractXMLValue(content, "TargetFramework"); tf != "" {
		builder.WriteString(fmt.Sprintf("TargetFramework: %s\n", tf))
	}
	if tfs := extractXMLValue(content, "TargetFrameworks"); tfs != "" {
		builder.WriteString(fmt.Sprintf("TargetFrameworks: %s\n", tfs))
	}

	// Count package references
	pkgPattern := regexp.MustCompile(`<PackageReference`)
	pkgs := pkgPattern.FindAllString(content, -1)
	if len(pkgs) > 0 {
		builder.WriteString(fmt.Sprintf("PackageReferences: %d\n", len(pkgs)))
	}
}

func (p *XMLParser) extractNuGetInfo(builder *strings.Builder, content string) {
	if id := extractXMLValue(content, "id"); id != "" {
		builder.WriteString(fmt.Sprintf("Package ID: %s\n", id))
	}
	if version := extractXMLValue(content, "version"); version != "" {
		builder.WriteString(fmt.Sprintf("Version: %s\n", version))
	}
	if authors := extractXMLValue(content, "authors"); authors != "" {
		builder.WriteString(fmt.Sprintf("Authors: %s\n", authors))
	}
}

func (p *XMLParser) extractSVGInfo(builder *strings.Builder, content string) {
	if width := extractXMLAttr(content, "svg", "width"); width != "" {
		builder.WriteString(fmt.Sprintf("Width: %s\n", width))
	}
	if height := extractXMLAttr(content, "svg", "height"); height != "" {
		builder.WriteString(fmt.Sprintf("Height: %s\n", height))
	}
	if viewBox := extractXMLAttr(content, "svg", "viewBox"); viewBox != "" {
		builder.WriteString(fmt.Sprintf("ViewBox: %s\n", viewBox))
	}
}

func (p *XMLParser) extractSchemaInfo(builder *strings.Builder, content string) {
	// Count elements and types
	elemPattern := regexp.MustCompile(`<(?:xs|xsd):element`)
	elems := elemPattern.FindAllString(content, -1)
	if len(elems) > 0 {
		builder.WriteString(fmt.Sprintf("Elements: %d\n", len(elems)))
	}

	typePattern := regexp.MustCompile(`<(?:xs|xsd):(?:complexType|simpleType)`)
	types := typePattern.FindAllString(content, -1)
	if len(types) > 0 {
		builder.WriteString(fmt.Sprintf("Types: %d\n", len(types)))
	}
}

// Regex patterns for XML symbol extraction with line tracking
var (
	xmlElementPattern       = regexp.MustCompile(`<(\w+)(?:\s|>|/)`)
	xmlDependencyPattern    = regexp.MustCompile(`<dependency>`)
	xmlGroupIdPattern       = regexp.MustCompile(`<groupId>([^<]+)</groupId>`)
	xmlArtifactIdPattern    = regexp.MustCompile(`<artifactId>([^<]+)</artifactId>`)
	xmlVersionPattern       = regexp.MustCompile(`<version>([^<]+)</version>`)
	xmlPackageRefPattern    = regexp.MustCompile(`<PackageReference\s+Include="([^"]+)"(?:\s+Version="([^"]+)")?`)
	xmlXsdElementPattern    = regexp.MustCompile(`<(?:xs|xsd):element\s+name="([^"]+)"`)
	xmlXsdComplexPattern    = regexp.MustCompile(`<(?:xs|xsd):complexType\s+name="([^"]+)"`)
	xmlPropertyGroupPattern = regexp.MustCompile(`<PropertyGroup`)
	xmlItemGroupPattern     = regexp.MustCompile(`<ItemGroup`)
	xmlTargetPattern        = regexp.MustCompile(`<Target\s+Name="([^"]+)"`)
)

func (p *XMLParser) extractSymbols(content string, xmlKind string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Track state for multi-line patterns
	var inDependency bool
	var depGroupId, depArtifactId, depVersion string
	var depStartLine int

	for lineNum, line := range lines {
		lineNo := lineNum + 1

		switch xmlKind {
		case "maven-pom":
			// Track dependency blocks
			if xmlDependencyPattern.MatchString(line) {
				inDependency = true
				depStartLine = lineNo
				depGroupId = ""
				depArtifactId = ""
				depVersion = ""
			}
			if inDependency {
				if matches := xmlGroupIdPattern.FindStringSubmatch(line); matches != nil {
					depGroupId = matches[1]
				}
				if matches := xmlArtifactIdPattern.FindStringSubmatch(line); matches != nil {
					depArtifactId = matches[1]
				}
				if matches := xmlVersionPattern.FindStringSubmatch(line); matches != nil {
					depVersion = matches[1]
				}
				if strings.Contains(line, "</dependency>") {
					if depGroupId != "" && depArtifactId != "" {
						symbols = append(symbols, Symbol{
							Name:     depGroupId + ":" + depArtifactId,
							Type:     "dependency",
							Line:     depStartLine,
							Value:    depVersion,
							Exported: true,
							Language: "xml",
						})
					}
					inDependency = false
				}
			}

		case "dotnet-project":
			// Extract package references
			if matches := xmlPackageRefPattern.FindStringSubmatch(line); matches != nil {
				version := ""
				if len(matches) > 2 {
					version = matches[2]
				}
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "package-reference",
					Line:     lineNo,
					Value:    version,
					Exported: true,
					Language: "xml",
				})
			}
			// Extract targets
			if matches := xmlTargetPattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "target",
					Line:     lineNo,
					Exported: true,
					Language: "xml",
				})
			}
			// Track PropertyGroup and ItemGroup sections
			if xmlPropertyGroupPattern.MatchString(line) {
				symbols = append(symbols, Symbol{
					Name:     "PropertyGroup",
					Type:     "section",
					Line:     lineNo,
					Exported: true,
					Language: "xml",
				})
			}
			if xmlItemGroupPattern.MatchString(line) {
				symbols = append(symbols, Symbol{
					Name:     "ItemGroup",
					Type:     "section",
					Line:     lineNo,
					Exported: true,
					Language: "xml",
				})
			}

		case "xsd-schema":
			// Extract element definitions
			if matches := xmlXsdElementPattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "element",
					Line:     lineNo,
					Exported: true,
					Language: "xml",
				})
			}
			// Extract complex type definitions
			if matches := xmlXsdComplexPattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "type",
					Line:     lineNo,
					Exported: true,
					Language: "xml",
				})
			}

		default:
			// Generic XML: extract top-level elements
			if strings.HasPrefix(strings.TrimSpace(line), "<") && !strings.HasPrefix(strings.TrimSpace(line), "<?") && !strings.HasPrefix(strings.TrimSpace(line), "<!") && !strings.HasPrefix(strings.TrimSpace(line), "</") {
				if matches := xmlElementPattern.FindStringSubmatch(line); matches != nil {
					elemName := matches[1]
					// Only extract significant elements (not common structural ones)
					if !isCommonXMLElement(elemName) {
						symbols = append(symbols, Symbol{
							Name:     elemName,
							Type:     "element",
							Line:     lineNo,
							Exported: true,
							Language: "xml",
						})
					}
				}
			}
		}
	}

	return symbols
}

func isCommonXMLElement(name string) bool {
	common := map[string]bool{
		"xml": true, "root": true, "item": true, "items": true,
		"list": true, "data": true, "value": true, "entry": true,
	}
	return common[strings.ToLower(name)]
}

// Helper to extract value between XML tags
func extractXMLValue(content, tag string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`<%s>([^<]+)</%s>`, tag, tag))
	if match := pattern.FindStringSubmatch(content); match != nil {
		return strings.TrimSpace(match[1])
	}
	return ""
}

// Helper to extract XML attribute value
func extractXMLAttr(content, tag, attr string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`<%s[^>]*\s%s="([^"]+)"`, tag, attr))
	if match := pattern.FindStringSubmatch(content); match != nil {
		return match[1]
	}
	return ""
}
