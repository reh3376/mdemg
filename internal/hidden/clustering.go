package hidden

import (
	"log"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// DistanceMatrix holds a precomputed pairwise distance matrix in float32.
// Compute once, reuse across multiple DBSCAN runs with different eps values.
type DistanceMatrix struct {
	N    int
	Data [][]float32 // Data[i][j] = cosine distance between points i and j
}

// PrecomputeDistanceMatrix normalizes all vectors and computes pairwise cosine
// distances using parallel goroutines. The result can be passed to DBSCANWithMatrix
// multiple times with different eps/minSamples without recomputation.
func PrecomputeDistanceMatrix(points [][]float64) *DistanceMatrix {
	n := len(points)
	if n == 0 {
		return &DistanceMatrix{N: 0}
	}

	// Normalize all vectors so cosine distance = 1 - dot(a, b)
	normed := make([][]float64, n)
	for i := range points {
		normed[i] = normalizeForDistance(points[i])
	}

	// Allocate distance matrix
	data := make([][]float32, n)
	for i := range data {
		data[i] = make([]float32, n)
	}

	// Compute upper triangle in parallel, mirror to lower
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	log.Printf("[DBSCAN] Computing %d×%d distance matrix (%d pairs) with %d workers...",
		n, n, n*(n-1)/2, numWorkers)
	start := time.Now()

	var wg sync.WaitGroup
	var rowsDone atomic.Int64
	rowCh := make(chan int, n)
	for i := 0; i < n; i++ {
		rowCh <- i
	}
	close(rowCh)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rowCh {
				vi := normed[i]
				for j := i + 1; j < n; j++ {
					dist := float32(normalizedCosineDistance(vi, normed[j]))
					data[i][j] = dist
					data[j][i] = dist
				}
				done := rowsDone.Add(1)
				if done%1000 == 0 {
					log.Printf("[DBSCAN] Distance matrix: %d/%d rows (%.1f%%)", done, n, float64(done)*100/float64(n))
				}
			}
		}()
	}
	wg.Wait()

	log.Printf("[DBSCAN] Distance matrix complete in %v", time.Since(start))
	return &DistanceMatrix{N: n, Data: data}
}

// DBSCANWithMatrix runs DBSCAN using a precomputed distance matrix.
// This is much faster than DBSCAN() when running multiple iterations with
// different eps values (adaptive DBSCAN), since the distance matrix is computed once.
func DBSCANWithMatrix(dm *DistanceMatrix, eps float64, minSamples int) []int {
	n := dm.N
	if n == 0 {
		return nil
	}

	epsF32 := float32(eps)
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Build neighbor lists from precomputed distances (parallel)
	log.Printf("[DBSCAN] Building neighbor lists for eps=%.4f...", eps)
	nbStart := time.Now()
	neighbors := make([][]int, n)
	var wg sync.WaitGroup
	neighborCh := make(chan int, n)
	for i := 0; i < n; i++ {
		neighborCh <- i
	}
	close(neighborCh)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range neighborCh {
				var nb []int
				row := dm.Data[i]
				for j := 0; j < n; j++ {
					if j != i && row[j] <= epsF32 {
						nb = append(nb, j)
					}
				}
				neighbors[i] = nb
			}
		}()
	}
	wg.Wait()
	log.Printf("[DBSCAN] Neighbor lists built in %v", time.Since(nbStart))

	// Standard DBSCAN clustering using precomputed neighbors
	labels := dbscanCore(n, neighbors, minSamples)

	// Count clusters for logging
	clusterSet := make(map[int]bool)
	noiseCount := 0
	for _, l := range labels {
		if l == -1 {
			noiseCount++
		} else {
			clusterSet[l] = true
		}
	}
	log.Printf("[DBSCAN] eps=%.4f: %d clusters, %d noise points", eps, len(clusterSet), noiseCount)

	return labels
}

// DBSCAN implements density-based spatial clustering of applications with noise.
// It uses cosine distance (1 - cosine_similarity) as the distance metric.
// The implementation precomputes a full distance matrix using parallel goroutines
// for O(1) neighbor lookups during the clustering phase.
//
// For adaptive DBSCAN (multiple runs with different eps), use
// PrecomputeDistanceMatrix + DBSCANWithMatrix instead.
func DBSCAN(points [][]float64, eps float64, minSamples int) []int {
	dm := PrecomputeDistanceMatrix(points)
	return DBSCANWithMatrix(dm, eps, minSamples)
}

// dbscanCore runs the DBSCAN graph-traversal phase using precomputed neighbor lists.
func dbscanCore(n int, neighbors [][]int, minSamples int) []int {
	labels := make([]int, n)
	for i := range labels {
		labels[i] = -2 // undefined
	}

	clusterID := 0

	for i := 0; i < n; i++ {
		if labels[i] != -2 {
			continue // already processed
		}

		nb := neighbors[i]
		if len(nb) < minSamples {
			labels[i] = -1 // noise
			continue
		}

		// Start a new cluster
		labels[i] = clusterID

		// Use a set for O(1) membership checks in the seed queue
		inSeed := make(map[int]bool, len(nb)*2)
		inSeed[i] = true
		seedQueue := make([]int, 0, len(nb))
		for _, idx := range nb {
			if !inSeed[idx] {
				inSeed[idx] = true
				seedQueue = append(seedQueue, idx)
			}
		}

		for j := 0; j < len(seedQueue); j++ {
			q := seedQueue[j]
			if labels[q] == -1 {
				labels[q] = clusterID // change noise to border point
			}
			if labels[q] != -2 {
				continue // already processed
			}

			labels[q] = clusterID

			qNb := neighbors[q]
			if len(qNb) >= minSamples {
				for _, neighbor := range qNb {
					if !inSeed[neighbor] {
						inSeed[neighbor] = true
						seedQueue = append(seedQueue, neighbor)
					}
				}
			}
		}

		clusterID++
	}

	return labels
}

// normalizeForDistance returns a unit-length copy of the vector.
// For normalized vectors, cosine distance = 1 - dot(a,b).
func normalizeForDistance(v []float64) []float64 {
	if len(v) == 0 {
		return nil
	}
	var norm float64
	for _, val := range v {
		norm += val * val
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		result := make([]float64, len(v))
		copy(result, v)
		return result
	}
	result := make([]float64, len(v))
	for i, val := range v {
		result[i] = val / norm
	}
	return result
}

// normalizedCosineDistance computes cosine distance between two unit vectors.
// For unit vectors: cosine_distance = 1 - dot(a, b)
func normalizedCosineDistance(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 1.0
	}
	var dot float64
	for i := range a {
		dot += a[i] * b[i]
	}
	// Clamp dot product to [-1, 1] to avoid floating point artifacts
	if dot > 1.0 {
		dot = 1.0
	} else if dot < -1.0 {
		dot = -1.0
	}
	return 1.0 - dot
}

// cosineDistance computes 1 - cosine_similarity between two vectors
func cosineDistance(a, b []float64) float64 {
	sim := cosineSimilarity(a, b)
	return 1.0 - sim
}

// cosineSimilarity computes the cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ComputeCentroid calculates the centroid (element-wise mean) of multiple embeddings
func ComputeCentroid(embeddings [][]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}

	dim := len(embeddings[0])
	if dim == 0 {
		return nil
	}

	centroid := make([]float64, dim)
	count := 0

	for _, emb := range embeddings {
		if len(emb) != dim {
			continue // skip mismatched dimensions
		}
		for i, v := range emb {
			centroid[i] += v
		}
		count++
	}

	if count == 0 {
		return nil
	}

	for i := range centroid {
		centroid[i] /= float64(count)
	}

	return centroid
}

// NormalizeVector normalizes a vector to unit length
func NormalizeVector(v []float64) []float64 {
	if len(v) == 0 {
		return nil
	}

	var norm float64
	for _, val := range v {
		norm += val * val
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		return v
	}

	result := make([]float64, len(v))
	for i, val := range v {
		result[i] = val / norm
	}

	return result
}

// GroupByCluster groups base nodes by their cluster labels
func GroupByCluster(nodes []BaseNode, labels []int) (map[int][]BaseNode, []BaseNode) {
	clusters := make(map[int][]BaseNode)
	var noise []BaseNode

	for i, label := range labels {
		if label == -1 {
			noise = append(noise, nodes[i])
		} else {
			clusters[label] = append(clusters[label], nodes[i])
		}
	}

	return clusters, noise
}

// GroupByPathPrefix groups base nodes by their directory path prefix
// depth controls how many path segments to use (e.g., 2 = /dir1/dir2/)
func GroupByPathPrefix(nodes []BaseNode, depth int) map[string][]BaseNode {
	groups := make(map[string][]BaseNode)

	for _, node := range nodes {
		prefix := extractPathPrefix(node.Path, depth)
		groups[prefix] = append(groups[prefix], node)
	}

	return groups
}

// extractPathPrefix extracts the first N directory segments from a path
func extractPathPrefix(path string, depth int) string {
	if path == "" {
		return "_unknown_"
	}

	// Split path and collect first N segments
	segments := splitPath(path)
	if len(segments) <= depth {
		return path
	}

	// Join first N segments
	result := ""
	for i := 0; i < depth && i < len(segments); i++ {
		if segments[i] != "" {
			if result != "" {
				result += "/"
			}
			result += segments[i]
		}
	}

	if result == "" {
		return "_root_"
	}
	return result
}

// splitPath splits a path into segments, handling leading slashes
func splitPath(path string) []string {
	var segments []string
	current := ""

	for _, ch := range path {
		if ch == '/' {
			if current != "" {
				segments = append(segments, current)
				current = ""
			}
		} else if ch == '#' {
			// Stop at # which often marks a symbol within a file
			if current != "" {
				segments = append(segments, current)
			}
			break
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		segments = append(segments, current)
	}

	return segments
}

// KMeansCluster performs k-means clustering on embeddings using cosine distance.
// Uses k-means++ initialization with O(n×k) min-distance cache and parallel
// assignment steps for large datasets.
// Returns cluster assignments (0 to k-1) for each point.
func KMeansCluster(embeddings [][]float64, k int, maxIter int) []int {
	n := len(embeddings)
	if n == 0 || k <= 0 {
		return nil
	}
	if k >= n {
		labels := make([]int, n)
		for i := range labels {
			labels[i] = i
		}
		return labels
	}

	dim := len(embeddings[0])
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Normalize embeddings for fast cosine distance (1 - dot product)
	normed := make([][]float64, n)
	for i := range embeddings {
		normed[i] = normalizeForDistance(embeddings[i])
	}

	log.Printf("[KMeans] Initializing k=%d centroids from %d points (dim=%d)...", k, n, dim)
	initStart := time.Now()

	// --- k-means++ initialization with O(n×k) min-distance cache ---
	centroids := make([][]float64, k)

	// First centroid: pick first point
	centroids[0] = make([]float64, dim)
	copy(centroids[0], normed[0])

	// minDist[j] = min distance from point j to any chosen centroid so far
	minDist := make([]float64, n)
	for j := range minDist {
		minDist[j] = math.MaxFloat64
	}

	for i := 1; i < k; i++ {
		// Update minDist with distance to the newly added centroid (i-1)
		prev := centroids[i-1]
		for j := 0; j < n; j++ {
			d := normalizedCosineDistance(normed[j], prev)
			if d < minDist[j] {
				minDist[j] = d
			}
		}

		// Pick the point with the largest minDist (farthest-first traversal)
		bestIdx := 0
		bestDist := -1.0
		for j := 0; j < n; j++ {
			if minDist[j] > bestDist {
				bestDist = minDist[j]
				bestIdx = j
			}
		}

		centroids[i] = make([]float64, dim)
		copy(centroids[i], normed[bestIdx])

		if i%100 == 0 {
			log.Printf("[KMeans] Init: %d/%d centroids placed", i, k)
		}
	}
	log.Printf("[KMeans] Initialization complete in %v", time.Since(initStart))

	// --- Iterative refinement with parallel assignment ---
	labels := make([]int, n)
	iterStart := time.Now()

	for iter := 0; iter < maxIter; iter++ {
		changed := int64(0)

		// Parallel assignment: each worker handles a chunk of points
		var wg sync.WaitGroup
		chunkSize := (n + numWorkers - 1) / numWorkers

		for w := 0; w < numWorkers; w++ {
			start := w * chunkSize
			end := start + chunkSize
			if end > n {
				end = n
			}
			if start >= n {
				break
			}

			wg.Add(1)
			go func(lo, hi int) {
				defer wg.Done()
				localChanged := int64(0)
				for i := lo; i < hi; i++ {
					minD := math.MaxFloat64
					minC := 0
					pt := normed[i]
					for c := 0; c < k; c++ {
						d := normalizedCosineDistance(pt, centroids[c])
						if d < minD {
							minD = d
							minC = c
						}
					}
					if labels[i] != minC {
						labels[i] = minC
						localChanged++
					}
				}
				atomic.AddInt64(&changed, localChanged)
			}(start, end)
		}
		wg.Wait()

		if changed == 0 {
			log.Printf("[KMeans] Converged at iteration %d", iter)
			break
		}

		if (iter+1)%10 == 0 {
			log.Printf("[KMeans] Iteration %d: %d points reassigned", iter+1, changed)
		}

		// Update centroids (sequential — accumulate sums)
		// Use incremental sum to avoid O(n×k) collect step
		sums := make([][]float64, k)
		counts := make([]int, k)
		for c := 0; c < k; c++ {
			sums[c] = make([]float64, dim)
		}
		for i := 0; i < n; i++ {
			c := labels[i]
			counts[c]++
			for d := 0; d < dim; d++ {
				sums[c][d] += normed[i][d]
			}
		}
		for c := 0; c < k; c++ {
			if counts[c] > 0 {
				for d := 0; d < dim; d++ {
					centroids[c][d] = sums[c][d] / float64(counts[c])
				}
				// Re-normalize centroid for cosine distance
				centroids[c] = normalizeForDistance(centroids[c])
			}
		}
	}

	log.Printf("[KMeans] Complete in %v (%d iterations)", time.Since(iterStart), maxIter)
	return labels
}

// ClassifyByExtension groups BaseNodes by normalized file extension category.
// Returns a map from category name to nodes in that category.
// Categories: "go", "typescript", "python", "markdown", "config", "shell", "sql", "other"
func ClassifyByExtension(nodes []BaseNode) map[string][]BaseNode {
	classes := make(map[string][]BaseNode)

	for _, node := range nodes {
		cat := classifyExtension(node.Path)
		classes[cat] = append(classes[cat], node)
	}

	return classes
}

// classifyExtension maps a file path to a semantic category.
func classifyExtension(path string) string {
	// Extract extension — handle paths like "file.go" and compound names like "file.go#Method"
	ext := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext = path[i+1:]
			break
		}
		if path[i] == '/' {
			break
		}
	}

	// Strip fragment after # (e.g., "go#ParseFile" → "go")
	for i := 0; i < len(ext); i++ {
		if ext[i] == '#' {
			ext = ext[:i]
			break
		}
	}

	switch ext {
	case "go":
		return "go"
	case "ts", "tsx", "js", "jsx", "mjs", "cjs":
		return "typescript"
	case "py", "pyi":
		return "python"
	case "md", "mdx", "rst", "txt":
		return "markdown"
	case "yaml", "yml", "json", "toml", "ini", "env", "cfg":
		return "config"
	case "sh", "bash", "zsh", "fish":
		return "shell"
	case "sql", "cypher":
		return "query"
	case "rs":
		return "rust"
	case "java", "kt", "kts":
		return "jvm"
	case "c", "cpp", "cc", "h", "hpp":
		return "cpp"
	case "css", "scss", "less", "html", "svg":
		return "web"
	case "proto", "graphql", "gql":
		return "schema"
	case "dockerfile", "containerfile":
		return "container"
	default:
		if ext == "" {
			return "other"
		}
		return "other"
	}
}

// SplitLargeCluster splits a cluster that exceeds maxSize
// Uses simple chunking since k-means struggles with highly similar embeddings
func SplitLargeCluster(nodes []BaseNode, maxSize int) [][]BaseNode {
	if len(nodes) <= maxSize {
		return [][]BaseNode{nodes}
	}

	// Simple chunking approach - more reliable than k-means when embeddings are similar
	var result [][]BaseNode
	for i := 0; i < len(nodes); i += maxSize {
		end := i + maxSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]
		if len(chunk) > 0 {
			result = append(result, chunk)
		}
	}

	return result
}
