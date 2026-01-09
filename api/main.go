package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	opensearch "github.com/opensearch-project/opensearch-go/v2"
	opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"github.com/redis/go-redis/v9"
)

const IndexName = "sanctions-v1"

// --- TYPES ---
type PaymentRequest struct {
	ReceiverName string `json:"receiver_name"`
}
type ScreeningResponse struct {
	Verdict   string  `json:"verdict"`
	RiskScore float64 `json:"risk_score"`
	Hits      []Hit   `json:"hits"`
}
type Hit struct {
	SanctionedName string  `json:"sanctioned_name"`
	ListSource     string  `json:"list_source"`
	Score          float64 `json:"score"`
	MatchMethod    string  `json:"match_method"`
}
type OpenSearchResponse struct {
	Hits struct {
		Hits []struct {
			Score  float64 `json:"_score"`
			Source struct {
				FullName   string `json:"full_name"`
				ListSource string `json:"list_source"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

var osClient *opensearch.Client
var rdb *redis.Client

func main() {
	ctx := context.Background()

	// 1. Redis
	rdb = redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})

	// 2. OpenSearch Connection (WITH ROBUST RETRY)
	var err error
	fmt.Println("⏳ Waiting for OpenSearch to be fully healthy...")
	
	for i := 0; i < 60; i++ { // Retry for 60 seconds
		osClient, err = opensearch.NewClient(opensearch.Config{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			Addresses: []string{os.Getenv("OPENSEARCH_URL")},
		})
		
		if err == nil {
			// CRITICAL FIX: We must check res.IsError()!
			// If server returns 503 (Security Initializing), res.IsError() is true.
			res, infoErr := osClient.Info()
			if infoErr == nil && !res.IsError() {
				fmt.Println("✅ OpenSearch is READY (200 OK)")
				res.Body.Close()
				break
			}
			if res != nil {
				res.Body.Close()
			}
		}
		
		fmt.Printf("zzZ OpenSearch not ready yet... (%d/60)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	// 3. Create Index (With FAISS and Phonetic)
	ensureIndexExists(ctx)

	// 4. Start Server
	http.HandleFunc("/screen", handleScreening)
	fmt.Println("🚀 Sentinel Engine listening on :8080")
	http.ListenAndServe(":8080", nil)
}

func handleScreening(w http.ResponseWriter, r *http.Request) {
	var req PaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}

	// Step A: Cache Check
	cacheKey := "safe_entity:" + strings.ToLower(strings.ReplaceAll(req.ReceiverName, " ", ""))
	if val, _ := rdb.Get(context.Background(), cacheKey).Result(); val == "CLEAR" {
		json.NewEncoder(w).Encode(ScreeningResponse{Verdict: "CLEAR", RiskScore: 0.0, Hits: []Hit{}})
		return
	}

	// Step B: Search (Multi-Match)
	mockVector := make([]float32, 384)
	for i := range mockVector { mockVector[i] = rand.Float32() }
	vectorJSON, _ := json.Marshal(mockVector)

	// Search Query
	query := fmt.Sprintf(`{
		"size": 5,
		"query": {
			"bool": {
				"should": [
					{ "match": { "full_name": { "query": "%s", "boost": 3.0, "operator": "or" } } },
					{ "match": { "full_name_phonetic": { "query": "%s", "fuzziness": "AUTO" } } },
					{ "knn": { "name_vector": { "vector": %s, "k": 2 } } }
				],
				"minimum_should_match": 1
			}
		}
	}`, req.ReceiverName, req.ReceiverName, string(vectorJSON))

	searchRes, err := osClient.Search(
		osClient.Search.WithIndex(IndexName),
		osClient.Search.WithBody(strings.NewReader(query)),
	)
	
	if err != nil {
		log.Printf("Search Error: %v", err)
		http.Error(w, "DB Error", 500)
		return
	}
	defer searchRes.Body.Close()

	var osResp OpenSearchResponse
	if err := json.NewDecoder(searchRes.Body).Decode(&osResp); err != nil {
		log.Printf("JSON Parse Error: %v", err)
		http.Error(w, "Parse Error", 500)
		return
	}

	verdict := "CLEAR"
	riskScore := 0.01
	var hits []Hit
	
	if len(osResp.Hits.Hits) > 0 {
		verdict = "REVIEW"
		riskScore = 0.85
		for _, h := range osResp.Hits.Hits {
			hits = append(hits, Hit{
				SanctionedName: h.Source.FullName,
				ListSource:     h.Source.ListSource,
				Score:          h.Score,
				MatchMethod:    "HYBRID",
			})
		}
	} else {
		hits = []Hit{} // Return empty array instead of null
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ScreeningResponse{Verdict: verdict, RiskScore: riskScore, Hits: hits})
}

func ensureIndexExists(ctx context.Context) {
	// Check if index exists
	req := opensearchapi.IndicesExistsRequest{Index: []string{IndexName}}
	res, err := req.Do(ctx, osClient)
	if err == nil && res.StatusCode == 200 { 
		return 
	}

	fmt.Println("⚠️ Creating Index with FAISS + Phonetic...")
	mapping := `{
		"settings": {
			"index.knn": true,
			"analysis": {
				"filter": { "my_metaphone": { "type": "phonetic", "encoder": "double_metaphone" } },
				"analyzer": { "phonetic_analyzer": { "tokenizer": "standard", "filter": ["lowercase", "my_metaphone"] } }
			}
		},
		"mappings": {
			"properties": {
				"full_name": { "type": "text" },
				"full_name_phonetic": { "type": "text", "analyzer": "phonetic_analyzer" },
				"name_vector": { "type": "knn_vector", "dimension": 384, "method": { "name": "hnsw", "engine": "faiss" } }
			}
		}
	}`
	createReq := opensearchapi.IndicesCreateRequest{Index: IndexName, Body: strings.NewReader(mapping)}
	createRes, err := createReq.Do(ctx, osClient)
	if err != nil || createRes.IsError() { 
		log.Printf("❌ Index Create Failed: %s", createRes) 
	} else {
		fmt.Println("✅ Index Created Successfully!")
	}
}
