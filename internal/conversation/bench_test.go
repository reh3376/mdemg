package conversation

import (
	"fmt"
	"testing"
)

// BenchmarkScoreObservationQuality benchmarks the quality scoring function.
func BenchmarkScoreObservationQuality(b *testing.B) {
	content := "Use Neo4j vector index memNodeEmbedding for similarity search. Set dimensions=1536 for OpenAI text-embedding-3-small. This provides fast approximate nearest neighbor lookups with cosine similarity."
	tags := []string{"neo4j", "vectors", "embeddings"}
	metadata := map[string]any{"file": "config.go", "decision": "vector-index"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScoreObservationQuality(content, "decision", tags, metadata)
	}
}

// BenchmarkScoreSpecificity benchmarks the specificity sub-scorer.
func BenchmarkScoreSpecificity(b *testing.B) {
	benchmarks := []struct {
		name    string
		content string
	}{
		{"short", "something happened"},
		{"medium", "The SessionTracker uses sync.Map with a 2-hour TTL for cleanup of stale sessions"},
		{"long", "Build failed in internal/api/middleware.go:42 - undefined: SessionResumeWarningMiddleware. Need to implement the function body before it can be referenced in server.go Routes(). The middleware should check request paths /v1/memory/retrieve and /v1/conversation/recall, extract session_id from the JSON body, and add an X-MDEMG-Warning header if the session has not called /resume."},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				scoreSpecificity(bm.content)
			}
		})
	}
}

// BenchmarkGenerateSummary benchmarks the summary generation function.
func BenchmarkGenerateSummary(b *testing.B) {
	content := "This is a moderately long observation that describes a decision made during development. The team chose to use Neo4j's vector index for semantic similarity search because it provides native graph traversal alongside vector operations. This avoids the need for a separate vector database like Pinecone or Milvus."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateSummary(content)
	}
}

// BenchmarkResumeObsTypePriority benchmarks the type priority lookup.
func BenchmarkResumeObsTypePriority(b *testing.B) {
	types := []ObservationType{
		ObsTypeCorrection, ObsTypeDecision, ObsTypeLearning,
		ObsTypePreference, ObsTypeError, ObsTypeTask,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resumeObsTypePriority(types[i%len(types)])
	}
}

// BenchmarkBuildObservationTags benchmarks tag building.
func BenchmarkBuildObservationTags(b *testing.B) {
	req := ObserveRequest{
		SessionID: "session-123",
		Tags:      []string{"architecture", "database", "performance"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildObservationTags(req, ObsTypeDecision)
	}
}

// BenchmarkIsCodeIdentifier benchmarks code identifier detection.
func BenchmarkIsCodeIdentifier(b *testing.B) {
	words := []string{
		"SessionTracker", "camelCase", "sync.Map", "snake_case",
		"SCREAMING_CASE", "internal.api.server", "hello", "the",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isCodeIdentifier(words[i%len(words)])
	}
}

// BenchmarkDedupAction benchmarks the dedup action decision.
func BenchmarkDedupAction(b *testing.B) {
	similarities := []float64{0.80, 0.90, 0.95, 0.99, 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dedupAction(similarities[i%len(similarities)], DedupThreshold)
	}
}

// BenchmarkCosineSimilarity benchmarks cosine similarity computation.
func BenchmarkCosineSimilarity(b *testing.B) {
	dims := []int{384, 768, 1536}

	for _, dim := range dims {
		b.Run(fmt.Sprintf("dim=%d", dim), func(b *testing.B) {
			a := make([]float32, dim)
			bb := make([]float32, dim)
			for i := range a {
				a[i] = float32(i) / float32(dim)
				bb[i] = float32(dim-i) / float32(dim)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cosineSimilarity(a, bb)
			}
		})
	}
}
