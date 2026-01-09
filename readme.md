Sentin-AI Compliance Engine 🛡️A Cloud-Native, Hybrid-Search Sanctions Screening Architecture.Sentin-AI is a high-performance regulatory compliance engine designed to screen payment transactions against government watchlists (OFAC, EU, UN). It utilizes a Hybrid Search strategy combining Vector Embeddings (AI), Phonetic Matching, and Fuzzy Keyword Search to detect sanctioned entities with high accuracy and low latency.🚀 Key FeaturesHybrid Search Engine: Combines k-NN Vector Search (FAISS) with Phonetic (Double Metaphone) and Boolean Logic.Real-Time Screening: Sub-second response times (<200ms) for payment processing.Automated Data Pipeline: Apache Airflow DAGs automatically download, parse, and index daily OFAC XML updates.Resilient Architecture: Go microservices with robust retry logic, connection pooling, and Redis caching.Audit-Ready: Returns detailed "Evidence Packages" explaining exactly why a transaction was flagged (e.g., "98% Phonetic Match").🏗️ ArchitectureComponentTechnologyRoleAPI ServiceGo (Golang) 1.21High-performance REST API for screening requests.Search EngineOpenSearchStores vectors & text. Uses analysis-phonetic & faiss engine.CacheRedisCaches "Safe Entities" to bypass search for known good customers.ETL PipelineApache AirflowOrchestrates daily ingestion of OFAC SDN lists.InfrastructureDocker ComposeLocal orchestration with custom builds.📂 Project StructureEnsure your directory looks like this before starting:Plaintextsentin-ai-compliance/
├── api/
│   ├── Dockerfile          # Multi-stage Go build
│   ├── go.mod              # Go dependencies
│   └── main.go             # Core screening logic
├── airflow_custom/
│   └── Dockerfile          # Airflow image with opensearch-py
├── opensearch_custom/
│   └── Dockerfile          # OpenSearch image with Phonetic plugin
├── dags/
│   └── sanction_etl.py     # ETL Script for OFAC XML
├── docker-compose.yml      # Orchestration config
└── README.md
🛠️ Installation & SetupPrerequisitesDocker Desktop installed and running.Step 1: Clone & BuildBash# Clone the repository
git clone https://github.com/your-username/sentin-ai-compliance.git
cd sentin-ai-compliance

# Build and Start the Cluster
# This builds custom images for Airflow (Python libs) and OpenSearch (Plugins)
docker-compose up --build -d
Step 2: Verify ServicesWait ~1 minute for services to initialize.Go API: docker logs -f sentin-apiSuccess: ✅ OpenSearch is READY and ✅ Index Created Successfully!Airflow UI: http://localhost:8081 (Login: admin / admin)OpenSearch: https://localhost:9200 (User: admin / StrongP@ssw0rd!)Step 3: Ingest Data (ETL)The search engine starts empty. You must run the Airflow pipeline to download the OFAC list.Go to Airflow UI (http://localhost:8081).Find sanction_etl in the DAG list.Unpause the DAG (Toggle the switch to Blue).Click the Play Button (▶) under Actions -> "Trigger DAG".Wait for the task to turn Dark Green (Success).🧪 Usage & Testing1. Clear Transaction (Safe)Test a name that is not on a watchlist.Bashcurl -X POST http://localhost:8080/screen \
     -H "Content-Type: application/json" \
     -d '{
           "receiver_name": "John Smith"
         }'
Response:JSON{
    "verdict": "CLEAR",
    "risk_score": 0.01,
    "hits": []
}
2. Sanctioned Entity (Review)Test a known sanctioned entity. Note that we use "Nicolas Maduro" (First Last), but the database stores "MADURO MOROS, Nicolas" (Last, First). The system should still match it.Bashcurl -X POST http://localhost:8080/screen \
     -H "Content-Type: application/json" \
     -d '{
           "receiver_name": "Nicolas Maduro"
         }'
Response:JSON{
    "verdict": "REVIEW",
    "risk_score": 0.85,
    "hits": [
        {
            "sanctioned_name": "MADURO MOROS, Nicolas",
            "list_source": "OFAC-SDN",
            "score": 27.23,
            "match_method": "HYBRID"
        }
    ]
}
🔧 TroubleshootingError: index_not_found_exceptionCause: The Go API started before OpenSearch was ready.Fix: The current main.go has retry logic, but you can force a restart: docker restart sentin-api.Error: Map keys must be unique in docker-composeCause: Duplicate keys in YAML.Fix: Ensure you are using the latest docker-compose.yml provided in this repo.Error: missing go.sum entryCause: Dependency mismatch.Fix: The api/Dockerfile now includes RUN go mod tidy to fix this automatically during build.🗺️ RoadmapPhase 1 (Completed): Core Engine, ETL, Docker Infrastructure.Phase 2: React/Next.js Dashboard for Case Management.Phase 3: Terraform deployment to AWS (EKS + OpenSearch Service).📜 LicenseMIT License. Open Source Technologies Used: OpenSearch, Redis, Apache Airflow, Go.