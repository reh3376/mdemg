package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&CudaParser{})
}

// CudaParser implements LanguageParser for CUDA source files (.cu, .cuh)
type CudaParser struct{}

func (p *CudaParser) Name() string {
	return "cuda"
}

func (p *CudaParser) Extensions() []string {
	return []string{".cu", ".cuh"}
}

func (p *CudaParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".cu") || strings.HasSuffix(pathLower, ".cuh")
}

func (p *CudaParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "_test.") ||
		strings.Contains(pathLower, "_tests.") ||
		strings.Contains(pathLower, "/test/") ||
		strings.Contains(pathLower, "/tests/")
}

func (p *CudaParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect if header or implementation
	isHeader := strings.HasSuffix(path, ".cuh")

	// Extract CUDA-specific constructs
	kernels := p.extractKernels(content)
	deviceFuncs := p.extractDeviceFunctions(content)
	launchSites := p.extractLaunchSites(content)
	sharedMemory := p.extractSharedMemory(content)

	// Extract C++ base constructs
	namespaces := FindAllMatches(content, `namespace\s+(\w+)\s*\{`)
	classes := FindAllMatches(content, `class\s+(\w+)(?:\s*:\s*(?:public|private|protected)\s+[\w:]+)?(?:\s*\{)?`)
	structs := FindAllMatches(content, `struct\s+(\w+)(?:\s*\{)?`)
	includes := FindAllMatches(content, `#include\s*[<"]([^>"]+)[>"]`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("CUDA file: %s\n", fileName))

	if isHeader {
		contentBuilder.WriteString("Type: CUDA Header\n")
	} else {
		contentBuilder.WriteString("Type: CUDA Source\n")
	}

	if len(kernels) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Kernels (__global__): %s\n", strings.Join(kernels, ", ")))
	}
	if len(deviceFuncs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Device Functions (__device__): %s\n", strings.Join(deviceFuncs, ", ")))
	}
	if len(launchSites) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Kernel Launches: %d sites\n", len(launchSites)))
	}
	if len(sharedMemory) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Shared Memory (__shared__): %s\n", strings.Join(sharedMemory, ", ")))
	}
	if len(namespaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Namespaces: %s\n", strings.Join(uniqueStrings(namespaces), ", ")))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(structs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Structs: %s\n", strings.Join(uniqueStrings(structs), ", ")))
	}
	contentBuilder.WriteString(fmt.Sprintf("Includes: %d\n", len(includes)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)

	// Build tags
	tags := []string{"cuda", "gpu"}
	if isHeader {
		tags = append(tags, "header")
	} else {
		tags = append(tags, "implementation")
	}
	if len(kernels) > 0 {
		tags = append(tags, "has_kernels")
	}
	if len(launchSites) > 0 {
		tags = append(tags, "launch_site")
	}
	tags = append(tags, concerns...)

	// Determine file kind
	cudaKind := "cuda-source"
	if isHeader {
		cudaKind = "cuda-header"
	}

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:        fileName,
		Kind:        cudaKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     strings.Join(uniqueStrings(namespaces), "::"),
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	})

	// Add kernels as separate elements
	for _, kernel := range kernels {
		elements = append(elements, CodeElement{
			Name:        kernel,
			Kind:        "kernel",
			Path:        fmt.Sprintf("/%s#%s", relPath, kernel),
			Content:     fmt.Sprintf("CUDA kernel '%s' (__global__) in file %s", kernel, fileName),
			Package:     strings.Join(uniqueStrings(namespaces), "::"),
			FilePath:    relPath,
			Tags:        []string{"cuda", "kernel", "__global__"},
			Concerns:    concerns,
			ElementKind: "kernel",
		})
	}

	// Add device functions as separate elements
	for _, devFunc := range deviceFuncs {
		elements = append(elements, CodeElement{
			Name:        devFunc,
			Kind:        "device_function",
			Path:        fmt.Sprintf("/%s#%s", relPath, devFunc),
			Content:     fmt.Sprintf("CUDA device function '%s' (__device__) in file %s", devFunc, fileName),
			Package:     strings.Join(uniqueStrings(namespaces), "::"),
			FilePath:    relPath,
			Tags:        []string{"cuda", "device_function", "__device__"},
			Concerns:    concerns,
			ElementKind: "symbol",
		})
	}

	// Add classes as separate elements
	for _, class := range uniqueStrings(classes) {
		elements = append(elements, CodeElement{
			Name:        class,
			Kind:        "class",
			Path:        fmt.Sprintf("/%s#%s", relPath, class),
			Content:     fmt.Sprintf("Class '%s' in CUDA file %s", class, fileName),
			Package:     strings.Join(uniqueStrings(namespaces), "::"),
			FilePath:    relPath,
			Tags:        append([]string{"cuda", "class"}, concerns...),
			Concerns:    concerns,
			ElementKind: "symbol",
		})
	}

	return elements, nil
}

// extractKernels finds __global__ kernel functions
func (p *CudaParser) extractKernels(content string) []string {
	// Pattern: __global__ [optional return type] kernelName(
	pattern := regexp.MustCompile(`__global__\s+(?:void|[\w:]+)\s+(\w+)\s*\(`)
	matches := pattern.FindAllStringSubmatch(content, -1)

	var kernels []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			kernels = append(kernels, match[1])
			seen[match[1]] = true
		}
	}
	return kernels
}

// extractDeviceFunctions finds __device__ functions (not also __host__)
func (p *CudaParser) extractDeviceFunctions(content string) []string {
	// First find all lines with __device__
	lines := strings.Split(content, "\n")
	pattern := regexp.MustCompile(`__device__\s+(?:inline\s+)?([\w:]+(?:\s*[*&])?)\s+(\w+)\s*\(`)

	var funcs []string
	seen := make(map[string]bool)
	for _, line := range lines {
		// Skip __host__ __device__ functions
		if strings.Contains(line, "__host__") {
			continue
		}
		if matches := pattern.FindStringSubmatch(line); matches != nil {
			if len(matches) > 2 && !seen[matches[2]] {
				funcs = append(funcs, matches[2])
				seen[matches[2]] = true
			}
		}
	}
	return funcs
}

// extractLaunchSites finds kernel launch calls (<<<...>>>)
func (p *CudaParser) extractLaunchSites(content string) []string {
	// Pattern: kernelName<<<gridDim, blockDim, ...>>>
	pattern := regexp.MustCompile(`(\w+)\s*<<<[^>]+>>>`)
	matches := pattern.FindAllStringSubmatch(content, -1)

	var sites []string
	for _, match := range matches {
		if len(match) > 1 {
			sites = append(sites, match[1])
		}
	}
	return sites
}

// extractSharedMemory finds __shared__ variable declarations
func (p *CudaParser) extractSharedMemory(content string) []string {
	// Pattern: __shared__ type varName
	pattern := regexp.MustCompile(`__shared__\s+[\w:]+(?:\s*[*&])?\s+(\w+)`)
	matches := pattern.FindAllStringSubmatch(content, -1)

	var vars []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			vars = append(vars, match[1])
			seen[match[1]] = true
		}
	}
	return vars
}

func (p *CudaParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Pattern: __global__ kernel (may be multi-line, so don't require closing paren)
	kernelPattern := regexp.MustCompile(`^\s*__global__\s+(?:void|[\w:]+)\s+(\w+)\s*\(`)
	// Pattern: __device__ function (we'll filter out __host__ __device__ in code)
	devicePattern := regexp.MustCompile(`^\s*__device__\s+(?:inline\s+)?([\w:]+(?:\s*[*&])?)\s+(\w+)\s*\(([^)]*)\)`)
	// Pattern: __host__ __device__ function
	hostDevicePattern := regexp.MustCompile(`^\s*__host__\s+__device__\s+(?:inline\s+)?([\w:]+(?:\s*[*&])?)\s+(\w+)\s*\(([^)]*)\)`)
	// Pattern: __shared__ variable
	sharedPattern := regexp.MustCompile(`^\s*__shared__\s+([\w:]+(?:\s*[*&])?)\s+(\w+)`)
	// Pattern: #define NAME value
	definePattern := regexp.MustCompile(`^\s*#define\s+([A-Z][A-Z0-9_]*)\s+(.+)$`)
	// Pattern: const/constexpr
	constPattern := regexp.MustCompile(`^\s*(?:const|constexpr)\s+[\w:]+\s+([A-Z][A-Z0-9_]*)\s*=\s*(.+?);`)

	for i, line := range lines {
		lineNum := i + 1

		// Check for kernel
		if matches := kernelPattern.FindStringSubmatch(line); matches != nil {
			signature := fmt.Sprintf("__global__ void %s(...)", matches[1])
			symbols = append(symbols, Symbol{
				Name:      matches[1],
				Type:      "kernel",
				Signature: signature,
				Line:      lineNum,
				Exported:  true,
				Language:  "cuda",
			})
			continue
		}

		// Check for __host__ __device__
		if matches := hostDevicePattern.FindStringSubmatch(line); matches != nil {
			signature := fmt.Sprintf("__host__ __device__ %s %s(%s)", matches[1], matches[2], matches[3])
			if len(signature) > 150 {
				signature = signature[:150] + "..."
			}
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "function",
				Signature:      signature,
				TypeAnnotation: matches[1],
				Line:     lineNum,
				Exported:       true,
				Language:       "cuda",
			})
			continue
		}

		// Check for __device__ function (but not __host__ __device__)
		if !strings.Contains(line, "__host__") {
			if matches := devicePattern.FindStringSubmatch(line); matches != nil {
				signature := fmt.Sprintf("__device__ %s %s(%s)", matches[1], matches[2], matches[3])
				if len(signature) > 150 {
					signature = signature[:150] + "..."
				}
				symbols = append(symbols, Symbol{
					Name:           matches[2],
					Type:           "device_function",
					Signature:      signature,
					TypeAnnotation: matches[1],
					Line:           lineNum,
					Exported:       true,
					Language:       "cuda",
				})
				continue
			}
		}

		// Check for __shared__ variable
		if matches := sharedPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "variable",
				TypeAnnotation: matches[1] + " __shared__",
				Line:     lineNum,
				Exported:       false,
				Language:       "cuda",
			})
			continue
		}

		// Check for #define
		if matches := definePattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "macro",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				Line: lineNum,
				Exported:   true,
				Language:   "cuda",
			})
			continue
		}

		// Check for const
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "constant",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				Line: lineNum,
				Exported:   true,
				Language:   "cuda",
			})
		}
	}

	return symbols
}
