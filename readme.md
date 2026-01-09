# Sentin-AI Compliance Engine 🛡️

**A Cloud-Native, Hybrid-Search Sanctions Screening Architecture.**

Sentin-AI is a high-performance regulatory compliance engine designed to screen payment transactions against government watchlists (OFAC, EU, UN). It utilizes a **Hybrid Search** strategy combining **Vector Embeddings (AI)**, **Phonetic Matching**, and **Fuzzy Keyword Search** to detect sanctioned entities with high accuracy and low latency.

## 🚀 Key Features

* **Hybrid Search Engine:** Combines `k-NN` Vector Search (FAISS) with Phonetic (Double Metaphone) and Boolean Logic.
* **Real-Time Screening:** Sub-second response times (<200ms) for payment processing.
* **Automated Data Pipeline:** Apache Airflow DAGs automatically download, parse, and index daily OFAC XML updates.
* **Resilient Architecture:** Go microservices with robust retry logic, connection pooling, and Redis caching.
* **Audit-Ready:** Returns detailed "Evidence Packages" explaining exactly *why* a transaction was flagged (e.g., "98% Phonetic Match").

## 🏗️ Architecture

| Component | Technology | Role |
| --- | --- | --- |
| **API Service** | **Go (Golang) 1.21** | High-performance REST API for screening requests. |
| **Search Engine** | **OpenSearch** | Stores vectors & text. Uses `analysis-phonetic` & `faiss` engine. |
| **Cache** | **Redis** | Caches "Safe Entities" to bypass search for known good customers. |
| **ETL Pipeline** | **Apache Airflow** | Orchestrates daily ingestion of OFAC SDN lists. |
| **Infrastructure** | **Docker Compose** | Local orchestration with custom builds. |

---

## 📂 Project Structure

Ensure your directory looks like this before starting:

```text
sentin-ai-compliance/
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

```

---

## 🛠️ Installation & Setup

### Prerequisites

* Docker Desktop installed and running.

### Step 1: Clone & Build

```bash
# Clone the repository
git clone https://github.com/your-username/sentin-ai-compliance.git
cd sentin-ai-compliance

# Build and Start the Cluster
# This builds custom images for Airflow (Python libs) and OpenSearch (Plugins)
docker-compose up --build -d

```

### Step 2: Verify Services

Wait ~1 minute for services to initialize.

* **Go API:** `docker logs -f sentin-api`
* *Success:* `✅ OpenSearch is READY` and `✅ Index Created Successfully!`


* **Airflow UI:** [http://localhost:8081](https://www.google.com/search?q=http://localhost:8081) (Login: `admin` / `admin`)
* **OpenSearch:** [https://localhost:9200](https://www.google.com/search?q=https://localhost:9200) (User: `admin` / `StrongP@ssw0rd!`)

### Step 3: Ingest Data (ETL)

The search engine starts empty. You must run the Airflow pipeline to download the OFAC list.

1. Go to **Airflow UI** ([http://localhost:8081](https://www.google.com/search?q=http://localhost:8081)).
2. Find `sanction_etl` in the DAG list.
3. **Unpause** the DAG (Toggle the switch to Blue).
4. Click the **Play Button (▶)** under Actions -> "Trigger DAG".
5. Wait for the task to turn **Dark Green** (Success).

---

## 🧪 Usage & Testing

### 1. Clear Transaction (Safe)

Test a name that is **not** on a watchlist.

```bash
curl -X POST http://localhost:8080/screen \
     -H "Content-Type: application/json" \
     -d '{
           "receiver_name": "John Smith"
         }'

```

**Response:**

```json
{
    "verdict": "CLEAR",
    "risk_score": 0.01,
    "hits": []
}

```

### 2. Sanctioned Entity (Review)

Test a known sanctioned entity. Note that we use `"Nicolas Maduro"` (First Last), but the database stores `"MADURO MOROS, Nicolas"` (Last, First). The system should still match it.

```bash
curl -X POST http://localhost:8080/screen \
     -H "Content-Type: application/json" \
     -d '{
           "receiver_name": "Nicolas Maduro"
         }'

```

**Response:**

```json
{
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

```

---

## 🔧 Troubleshooting

**Error: `index_not_found_exception**`

* **Cause:** The Go API started before OpenSearch was ready.
* **Fix:** The current `main.go` has retry logic, but you can force a restart: `docker restart sentin-api`.

**Error: `Map keys must be unique` in docker-compose**

* **Cause:** Duplicate keys in YAML.
* **Fix:** Ensure you are using the latest `docker-compose.yml` provided in this repo.

**Error: `missing go.sum entry**`

* **Cause:** Dependency mismatch.
* **Fix:** The `api/Dockerfile` now includes `RUN go mod tidy` to fix this automatically during build.

---

## 🗺️ Roadmap

* **Phase 1 (Completed):** Core Engine, ETL, Docker Infrastructure.
* **Phase 2:** React/Next.js Dashboard for Case Management.
* **Phase 3:** Terraform deployment to AWS (EKS + OpenSearch Service).

---

## 📜 License

MIT License. Open Source Technologies Used: OpenSearch, Redis, Apache Airflow, Go.
