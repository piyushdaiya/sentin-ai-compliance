# Sentin-AI Compliance Engine (Phase 2: ISO 20022) 🛡️

**A Cloud-Native, Hybrid-Search Sanctions Screening Architecture.**

Sentin-AI is a high-performance regulatory compliance engine designed to screen **ISO 20022 payment messages** against government watchlists (OFAC, EU, UN). It implements **Targeted Screening** principles, parsing complex XML structures to apply specific matching logic (Exact, Fuzzy, Phonetic) to specific fields (BICs, Names, Addresses).

## 🚀 Key Features

* **ISO 20022 Native:** Parses `pacs.008` (Customer Credit Transfer), `pacs.009` (Financial Institution Credit Transfer), and `pain.001` (Customer Credit Transfer Initiation) messages directly.
* **Targeted Screening:**
  * **BICs:** Exact matching against sanctioned bank codes found in Agent blocks (Tables 9 & 14 of Guidelines).
  * **Names:** Phonetic + Boolean AND logic (prevents "John Smith" false positives) applied to Parties (Tables 5 & 6).
  * **Addresses:** Fuzzy keyword matching for sanctioned countries/cities (e.g., "Tehran") in structured and unstructured address blocks (Tables 7 & 8).
  * **Hybrid Search:** Combines `k-NN` Vector Search (FAISS) with OpenSearch Text Analysis.
  * **Automated Data Pipeline:** Apache Airflow DAGs ingest OFAC SDN lists daily, extracting BICs and Addresses for the search index.

---

## 🏗️ Architecture


| Component          | Technology         | Role                                                                                                                                        |
| ------------------ | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| **API Service**    | **Go (Golang)**    | High-performance REST API that parses XML trees and extracts relevant entities (Agents, Parties) for screening.                             |
| **Search Engine**  | **OpenSearch**     | Stores vectors & text. Uses`analysis-phonetic` & `faiss` engine. Configured with specific mappings for BICs (Keyword) and Addresses (Text). |
| **ETL Pipeline**   | **Apache Airflow** | Orchestrates daily ingestion of OFAC SDN lists, parsing XML to extract alternate names, addresses, and identification codes.                |
| **Infrastructure** | **Docker Compose** | Orchestration with custom builds to ensure dependencies (OpenSearch plugins, Go libraries) are pre-loaded.                                  |

---

## 📂 Project Structure

Ensure your directory looks like this:

```text
sentin-ai-compliance/
├── api/
│   ├── Dockerfile          # Multi-stage Go build
│   ├── go.mod              # Go dependencies
│   └── main.go             # Core ISO 20022 Parser & Screening Logic
├── airflow_custom/
│   └── Dockerfile          # Airflow image with opensearch-py
├── opensearch_custom/
│   └── Dockerfile          # OpenSearch image with Phonetic plugin
├── dags/
│   └── sanction_etl.py     # ETL Script for OFAC XML Extraction
├── docker-compose.yml      # Orchestration config
└── README.md

```

---

## 🚀 Execution Guide

### Step 1: Initialize Infrastructure

Build the containers with the updated ISO 20022 logic.

```bash
docker-compose up --build -d

```

### Step 2: Initialize Database Index

Run the `PUT` command to create the `sanctions-v1` index. This is critical as it defines specific mappings for:

* **`bics`**: Keyword type for exact matching.
* **`address`**: Text type for fuzzy matching.
* **`full_name`**: Hybrid type for phonetic/vector matching.

### Step 3: Run Data Pipeline

1. Open **Airflow** (`http://localhost:8081`).
2. Unpause and Trigger `sanction_etl`.
3. Wait for the task to complete. This will populate the index with the latest OFAC data, including the BICs and Addresses needed for the new screening logic.

---

## 🧪 Testing & Verification

Run the following `curl` commands to verify the **Targeted Screening** logic across different message types.

### 1. Test Name Matching (`pacs.008`)

**Scenario:** A Customer Credit Transfer (`pacs.008`) where the **Creditor Name** matches a sanctioned entity.

**Logic:** Verifies Phonetic + Boolean AND matching (Table 5).

```bash
curl -X POST http://localhost:8080/screen/iso20022 \
  -H "Content-Type: application/xml" \
  -d '<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
        <FIToFICstmrCdtTrf>
          <CdtTrfTxInf>
            <Cdtr>
              <Nm>Nicolas Maduro</Nm>
              <PstlAdr><Ctry>VE</Ctry></PstlAdr>
            </Cdtr>
          </CdtTrfTxInf>
        </FIToFICstmrCdtTrf>
      </Document>'

```

**Expected Result:**

```json
{"verdict": "REVIEW", "hits": [{"matched_field": "Name", "sanctioned_entity": "Nicolas Maduro", "score": 85}]}

```

### 2. Test Address Matching (`pain.001`)

**Scenario:** A Payment Initiation (`pain.001`) where the **Debtor Address** contains a sanctioned city ("Tehran").

**Logic:** Verifies Fuzzy/Embargo keyword matching on unstructured address lines (Table 8).

```bash
curl -X POST http://localhost:8080/screen/iso20022 \
  -H "Content-Type: application/xml" \
  -d '<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09">
        <CstmrCdtTrfInitn>
          <PmtInf>
            <Dbtr>
              <Nm>Generic Trading Co</Nm>
              <PstlAdr>
                <AdrLine>Tehran, Iran</AdrLine>
              </PstlAdr>
            </Dbtr>
          </PmtInf>
        </CstmrCdtTrfInitn>
      </Document>'

```

**Expected Result:**

```json
{"verdict": "REVIEW", "hits": [{"matched_field": "Address", "sanctioned_entity": "Address Match", "score": 70}]}

```

### 3. Test BIC Matching (`pacs.009`)

**Prerequisite:** Ensure `TESTBIC1` (or use - any BIC code from current OFAC List ex- DCBKKPPY)exists in the index (Insert it manually if needed for testing).
**Scenario:** A Financial Institution Transfer (`pacs.009`) where the **Creditor Agent BIC** matches a sanctioned bank.

**Logic:** Verifies Exact BIC matching on `BICFI` or `AnyBIC` tags (Table 14).

```bash
curl -X POST http://localhost:8080/screen/iso20022 \
  -H "Content-Type: application/xml" \
  -d '<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.08">
        <FICdtTrf>
          <CdtTrfTxInf>
            <CdtrAgt>
              <FinInstnId>
                <BICFI>TESTBIC1</BICFI>
              </FinInstnId>
            </CdtrAgt>
          </CdtTrfTxInf>
        </FICdtTrf>
      </Document>'

```

**Expected Result:**

```json
{"verdict": "REVIEW", "hits": [{"matched_field": "BIC", "sanctioned_entity": "TESTBIC1", "score": 100}]}

```

### 4. Test Clear Transaction

**Scenario:** A standard payment between two non-sanctioned entities.
**Logic:** Verifies that common names (like "John Smith") do NOT trigger false positives due to "OR" logic.

```bash
curl -X POST http://localhost:8080/screen/iso20022 \
  -H "Content-Type: application/xml" \
  -d '<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
        <FIToFICstmrCdtTrf>
          <CdtTrfTxInf>
            <Cdtr>
              <Nm>John Smith</Nm>
              <PstlAdr>
                <Ctry>US</Ctry>
                <TwnNm>New York</TwnNm>
              </PstlAdr>
            </Cdtr>
            <Dbtr>
               <Nm>Jane Doe</Nm>
            </Dbtr>
          </CdtTrfTxInf>
        </FIToFICstmrCdtTrf>
      </Document>'

```

**Expected Result:**

```json
{"verdict": "CLEAR", "hits": null}

```

---

## 📜 License

MIT License. Open Source Technologies Used: OpenSearch, Redis, Apache Airflow, Go.
