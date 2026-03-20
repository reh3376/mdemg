package summarize

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"mdemg/internal/llmclient"
)

// BenchmarkSingleSummarize benchmarks single file summarization.
func BenchmarkSingleSummarize(b *testing.B) {
	// Create mock server for consistent benchmarking
	server := createMockOpenAIServer(1)
	defer server.Close()

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		MaxTokens:      150,
		BatchSize:      1,
		TimeoutMs:      5000,
		CacheEnabled:   false, // Disable cache to measure raw performance
		CacheSize:      100,
		OpenAIAPIKey:   "benchmark-key",
		OpenAIEndpoint: server.URL,
	}

	svc, err := New(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}

	elem := CodeElement{
		Name:    "BenchmarkFunction",
		Kind:    "function",
		Path:    "/benchmark/test.go#BenchmarkFunction",
		Package: "benchmark",
		Content: generateSampleCode(500), // 500 char code sample
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.Summarize(ctx, elem)
	}
}

// BenchmarkBatchSummarize10 benchmarks batch summarization with 10 elements.
func BenchmarkBatchSummarize10(b *testing.B) {
	benchmarkBatchSize(b, 10)
}

// BenchmarkBatchSummarize50 benchmarks batch summarization with 50 elements.
func BenchmarkBatchSummarize50(b *testing.B) {
	benchmarkBatchSize(b, 50)
}

// BenchmarkBatchSummarize100 benchmarks batch summarization with 100 elements.
func BenchmarkBatchSummarize100(b *testing.B) {
	benchmarkBatchSize(b, 100)
}

func benchmarkBatchSize(b *testing.B, batchSize int) {
	// Create mock server
	server := createMockOpenAIServer(batchSize)
	defer server.Close()

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		MaxTokens:      150,
		BatchSize:      10, // Process 10 at a time
		TimeoutMs:      30000,
		CacheEnabled:   false,
		OpenAIAPIKey:   "benchmark-key",
		OpenAIEndpoint: server.URL,
	}

	svc, err := New(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}

	elements := make([]CodeElement, batchSize)
	for i := 0; i < batchSize; i++ {
		elements[i] = CodeElement{
			Name:    fmt.Sprintf("Element%d", i),
			Kind:    "function",
			Path:    fmt.Sprintf("/benchmark/file%d.go#Element%d", i, i),
			Package: "benchmark",
			Content: generateSampleCode(500),
		}
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.SummarizeBatch(ctx, elements)
	}
}

// BenchmarkCacheHit benchmarks cache hit performance.
func BenchmarkCacheHit(b *testing.B) {
	server := createMockOpenAIServer(1)
	defer server.Close()

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		MaxTokens:      150,
		BatchSize:      10,
		TimeoutMs:      5000,
		CacheEnabled:   true,
		CacheSize:      1000,
		OpenAIAPIKey:   "benchmark-key",
		OpenAIEndpoint: server.URL,
	}

	svc, err := New(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}

	elem := CodeElement{
		Name:    "CachedElement",
		Kind:    "function",
		Path:    "/benchmark/cached.go#CachedElement",
		Package: "benchmark",
		Content: generateSampleCode(500),
	}

	ctx := context.Background()

	// Prime the cache
	_ = svc.Summarize(ctx, elem)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.Summarize(ctx, elem)
	}
}

// BenchmarkCacheMiss benchmarks cache miss performance.
func BenchmarkCacheMiss(b *testing.B) {
	server := createMockOpenAIServer(1)
	defer server.Close()

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		MaxTokens:      150,
		BatchSize:      10,
		TimeoutMs:      5000,
		CacheEnabled:   true,
		CacheSize:      1000,
		OpenAIAPIKey:   "benchmark-key",
		OpenAIEndpoint: server.URL,
	}

	svc, err := New(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Each iteration uses a unique element to ensure cache miss
		elem := CodeElement{
			Name:    fmt.Sprintf("UniqueElement%d", i),
			Kind:    "function",
			Path:    fmt.Sprintf("/benchmark/unique%d.go#UniqueElement%d", i, i),
			Package: "benchmark",
			Content: generateSampleCode(500) + fmt.Sprintf("// iteration %d", i),
		}
		_ = svc.Summarize(ctx, elem)
	}
}

// BenchmarkCacheOperations benchmarks pure cache operations without API calls.
func BenchmarkCacheOperations(b *testing.B) {
	cache := newSummaryCache(10000)

	b.Run("Put", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cache.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("summary%d", i))
		}
	})

	// Prime cache for Get benchmark
	for i := 0; i < 5000; i++ {
		cache.Put(fmt.Sprintf("getkey%d", i), fmt.Sprintf("summary%d", i))
	}

	b.Run("Get/Hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = cache.Get(fmt.Sprintf("getkey%d", i%5000))
		}
	})

	b.Run("Get/Miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = cache.Get(fmt.Sprintf("misskey%d", i))
		}
	})
}

// BenchmarkPromptBuilding benchmarks the prompt building function.
func BenchmarkPromptBuilding(b *testing.B) {
	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		OpenAIAPIKey: "test-key",
	}

	svc, err := New(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}

	elements := make([]CodeElement, 10)
	for i := 0; i < 10; i++ {
		elements[i] = CodeElement{
			Name:     fmt.Sprintf("Element%d", i),
			Kind:     "function",
			Path:     fmt.Sprintf("/path/file%d.go#Element%d", i, i),
			Package:  "testpkg",
			Content:  generateSampleCode(1000),
			Concerns: []string{"auth", "logging"},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.buildPrompt(elements)
	}
}

// BenchmarkCacheKeyGeneration benchmarks cache key generation.
func BenchmarkCacheKeyGeneration(b *testing.B) {
	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		OpenAIAPIKey: "test-key",
	}

	svc, err := New(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}

	elem := CodeElement{
		Name:    "TestElement",
		Kind:    "function",
		Path:    "/path/test.go#TestElement",
		Content: generateSampleCode(2000),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.cacheKey(elem)
	}
}

// BenchmarkRealOpenAI benchmarks real OpenAI API calls (skipped by default).
func BenchmarkRealOpenAI(b *testing.B) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		b.Skip("OPENAI_API_KEY not set, skipping real API benchmark")
	}

	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		MaxTokens:    150,
		BatchSize:    5,
		TimeoutMs:    30000,
		CacheEnabled: false,
		OpenAIAPIKey: apiKey,
	}

	svc, err := New(cfg, nil)
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}

	elem := CodeElement{
		Name:    "RealBenchmark",
		Kind:    "function",
		Path:    "/benchmark/real.go#RealBenchmark",
		Package: "benchmark",
		Content: `
func ProcessData(ctx context.Context, data []byte) (Result, error) {
    // Parse input data
    var input InputData
    if err := json.Unmarshal(data, &input); err != nil {
        return Result{}, fmt.Errorf("parse input: %w", err)
    }

    // Validate
    if err := input.Validate(); err != nil {
        return Result{}, fmt.Errorf("validate: %w", err)
    }

    // Process
    result := process(input)
    return result, nil
}`,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.Summarize(ctx, elem)
		// Add small delay to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}
}

// createMockOpenAIServer creates a mock server that returns valid responses.
func createMockOpenAIServer(expectedBatchSize int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time (but keep it fast for benchmarks)
		time.Sleep(1 * time.Millisecond)

		// Generate response array
		summaries := make([]string, expectedBatchSize)
		for i := 0; i < expectedBatchSize; i++ {
			summaries[i] = fmt.Sprintf("Mock summary for element %d: Processes data and returns results.", i)
		}

		summariesJSON, _ := json.Marshal(summaries)

		resp := openAIChatResponse{
			Choices: []llmclient.OpenAIChoice{
				{Message: llmclient.Message{Content: string(summariesJSON)}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

// generateSampleCode generates sample code of approximately the specified length.
func generateSampleCode(length int) string {
	template := `
func ProcessItem(ctx context.Context, item Item) (Result, error) {
    // Validate input
    if item.ID == "" {
        return Result{}, errors.New("item ID is required")
    }

    // Fetch dependencies
    deps, err := fetchDependencies(ctx, item.ID)
    if err != nil {
        return Result{}, fmt.Errorf("fetch deps: %w", err)
    }

    // Process
    result := Result{
        ID:        item.ID,
        Status:    "processed",
        Timestamp: time.Now(),
    }

    for _, dep := range deps {
        result.Dependencies = append(result.Dependencies, dep.Name)
    }

    return result, nil
}
`

	result := template
	for len(result) < length {
		result += "\n" + template
	}

	if len(result) > length {
		result = result[:length]
	}

	return result
}

// TestBenchmarkMetrics runs a quick performance test and reports metrics.
// This is a test (not benchmark) to generate human-readable output.
func TestBenchmarkMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark metrics in short mode")
	}

	server := createMockOpenAIServer(10)
	defer server.Close()

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		MaxTokens:      150,
		BatchSize:      10,
		TimeoutMs:      5000,
		CacheEnabled:   true,
		CacheSize:      100,
		OpenAIAPIKey:   "test-key",
		OpenAIEndpoint: server.URL,
	}

	svc, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elements := make([]CodeElement, 100)
	for i := 0; i < 100; i++ {
		elements[i] = CodeElement{
			Name:    fmt.Sprintf("Element%d", i),
			Kind:    "function",
			Path:    fmt.Sprintf("/test/file%d.go#Element%d", i, i),
			Package: "test",
			Content: generateSampleCode(500),
		}
	}

	ctx := context.Background()

	// Measure batch processing
	start := time.Now()
	summaries := svc.SummarizeBatch(ctx, elements)
	batchDuration := time.Since(start)

	t.Logf("Batch processing 100 elements: %v (%.2f ms/element)",
		batchDuration, float64(batchDuration.Milliseconds())/100.0)

	// Verify results
	if len(summaries) != 100 {
		t.Errorf("Expected 100 summaries, got %d", len(summaries))
	}

	// Measure cache hit performance
	start = time.Now()
	iterations := 1000
	for i := 0; i < iterations; i++ {
		_ = svc.Summarize(ctx, elements[i%100])
	}
	cacheDuration := time.Since(start)

	t.Logf("Cache hit performance: %d iterations in %v (%.2f us/hit)",
		iterations, cacheDuration, float64(cacheDuration.Microseconds())/float64(iterations))

	// Report stats
	totalCalls, cacheHits, cacheSize := svc.Stats()
	t.Logf("Final stats: totalCalls=%d, cacheHits=%d, cacheSize=%d, hitRate=%.2f%%",
		totalCalls, cacheHits, cacheSize, float64(cacheHits)/float64(totalCalls)*100)
}
