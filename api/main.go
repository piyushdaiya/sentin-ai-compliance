package main

import (
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	opensearch "github.com/opensearch-project/opensearch-go/v2"
)

const IndexName = "sanctions-v1"

// --- ISO 20022 XML STRUCTURES ---
type IsoMessage struct {
	XMLName xml.Name
	Pacs008 *DocumentBlock `xml:"FIToFICstmrCdtTrf"`
	Pacs009 *DocumentBlock `xml:"FICdtTrf"`
	Pain001 *DocumentBlock `xml:"CstmrCdtTrfInitn"`
}

type DocumentBlock struct {
	TxInf  []TransactionInformation `xml:"CdtTrfTxInf"`
	PmtInf []PaymentInformation     `xml:"PmtInf"`
}

type PaymentInformation struct {
	Dbtr    Party                    `xml:"Dbtr"`
	DbtrAgt Agent                    `xml:"DbtrAgt"`
	TxInf   []TransactionInformation `xml:"CdtTrfTxInf"`
}

type TransactionInformation struct {
	Cdtr    Party `xml:"Cdtr"`
	CdtrAgt Agent `xml:"CdtrAgt"`
}

type Party struct {
	Nm      string        `xml:"Nm"`
	PstlAdr PostalAddress `xml:"PstlAdr"`
	Id      PartyId       `xml:"Id"`
}

type Agent struct {
	FinInstnId FinancialInstitutionId `xml:"FinInstnId"`
}

type FinancialInstitutionId struct {
	BICFI   string        `xml:"BICFI"`
	Nm      string        `xml:"Nm"`
	PstlAdr PostalAddress `xml:"PstlAdr"`
}

type PartyId struct {
	OrgId OrganisationId `xml:"OrgId"`
}

type OrganisationId struct {
	AnyBIC string `xml:"AnyBIC"`
	LEI    string `xml:"LEI"`
}

type PostalAddress struct {
	Ctry    string   `xml:"Ctry"`
	TwnNm   string   `xml:"TwnNm"`
	AdrLine []string `xml:"AdrLine"`
}

type ScreenableEntity struct {
	Role    string
	Name    string
	Address string
	BIC     string
	Country string
}

type Hit struct {
	MatchedField   string  `json:"matched_field"`
	SanctionedName string  `json:"sanctioned_entity"`
	Score          float64 `json:"score"`
}

var osClient *opensearch.Client

func main() {
	// 1. Connection with Retry
	var err error
	for i := 0; i < 60; i++ {
		osClient, err = opensearch.NewClient(opensearch.Config{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			Addresses: []string{os.Getenv("OPENSEARCH_URL")},
		})
		if err == nil {
			res, infoErr := osClient.Info()
			if infoErr == nil && !res.IsError() {
				fmt.Println("✅ OpenSearch is READY")
				res.Body.Close()
				break
			}
			if res != nil { res.Body.Close() }
		}
		fmt.Printf("⏳ Waiting for OpenSearch... (%d/60)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	// 2. Start Server
	http.HandleFunc("/screen/iso20022", handleISO20022)
	fmt.Println("🚀 ISO 20022 Engine Listening on :8080")
	http.ListenAndServe(":8080", nil)
}

func handleISO20022(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var isoMsg IsoMessage
	if err := xml.Unmarshal(body, &isoMsg); err != nil {
		fmt.Printf("❌ XML Parse Error: %v\n", err)
		http.Error(w, "Invalid XML", 400)
		return
	}

	entities := extractEntities(&isoMsg)
	var allHits []Hit

	for _, ent := range entities {
		fmt.Printf("👉 Screening: Name='%s', BIC='%s', Addr='%s'\n", ent.Name, ent.BIC, ent.Address)
		hits := screenEntity(ent)
		allHits = append(allHits, hits...)
	}

	verdict := "CLEAR"
	if len(allHits) > 0 { verdict = "REVIEW" }

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"verdict": verdict,
		"hits":    allHits,
	})
}

func extractEntities(msg *IsoMessage) []ScreenableEntity {
	var list []ScreenableEntity

	addParty := func(p Party, role string) {
		if p.Nm == "" && p.Id.OrgId.AnyBIC == "" { return }
		addrLines := strings.Join(p.PstlAdr.AdrLine, " ")
		list = append(list, ScreenableEntity{
			Role: role, Name: p.Nm, Address: addrLines, Country: p.PstlAdr.Ctry,
			BIC: strings.ToUpper(strings.TrimSpace(p.Id.OrgId.AnyBIC)),
		})
	}

	addAgent := func(a Agent, role string) {
		if a.FinInstnId.BICFI == "" && a.FinInstnId.Nm == "" { return }
		list = append(list, ScreenableEntity{
			Role: role, Name: a.FinInstnId.Nm, Country: a.FinInstnId.PstlAdr.Ctry,
			BIC: strings.ToUpper(strings.TrimSpace(a.FinInstnId.BICFI)),
		})
	}

	if msg.Pacs008 != nil {
		for _, tx := range msg.Pacs008.TxInf {
			addParty(tx.Cdtr, "Creditor")
			addAgent(tx.CdtrAgt, "CreditorAgent")
		}
	}
	if msg.Pacs009 != nil {
		for _, tx := range msg.Pacs009.TxInf {
			addAgent(tx.CdtrAgt, "CreditorAgent")
		}
	}
	if msg.Pain001 != nil {
		for _, pmt := range msg.Pain001.PmtInf {
			addParty(pmt.Dbtr, "Debtor")
			addAgent(pmt.DbtrAgt, "DebtorAgent")
			for _, tx := range pmt.TxInf {
				addParty(tx.Cdtr, "Creditor")
				addAgent(tx.CdtrAgt, "CreditorAgent")
			}
		}
	}
	return list
}

func screenEntity(ent ScreenableEntity) []Hit {
	var hits []Hit

	// 1. BIC SCREENING
	if ent.BIC != "" {
		q := fmt.Sprintf(`{"query": {"term": {"bics": "%s"}}}`, ent.BIC)
		if executeSearch(q) {
			hits = append(hits, Hit{MatchedField: "BIC", SanctionedName: ent.BIC, Score: 100.0})
		}
	}

	// 2. NAME SCREENING
	if ent.Name != "" {
		q := fmt.Sprintf(`{
			"query": {
				"bool": {
					"should": [
						{ "match": { "full_name": { "query": "%s", "fuzziness": "AUTO", "operator": "and" }}},
						{ "match": { "alt_names": { "query": "%s", "fuzziness": "AUTO", "operator": "and" }}}
					],
					"minimum_should_match": 1
				}
			}
		}`, ent.Name, ent.Name)

		if executeSearch(q) {
			hits = append(hits, Hit{MatchedField: "Name", SanctionedName: ent.Name, Score: 85.0})
		}
	}

	// 3. ADDRESS SCREENING
	if ent.Address != "" {
		q := fmt.Sprintf(`{ "query": { "match": { "address": { "query": "%s", "fuzziness": "AUTO" }}}}`, ent.Address)
		if executeSearch(q) {
			hits = append(hits, Hit{MatchedField: "Address", SanctionedName: "Address Match", Score: 70.0})
		}
	}

	return hits
}

func executeSearch(queryJson string) bool {
	req := strings.NewReader(queryJson)
	res, err := osClient.Search(
		osClient.Search.WithIndex(IndexName),
		osClient.Search.WithBody(req),
	)
	if err != nil {
		fmt.Printf("DB Error: %v\n", err)
		return false
	}
	defer res.Body.Close()

	var r map[string]interface{}
	json.NewDecoder(res.Body).Decode(&r)

	if hits, ok := r["hits"].(map[string]interface{}); ok {
		if total, ok := hits["total"].(map[string]interface{}); ok {
			if val, ok := total["value"].(float64); ok {
				return val > 0
			}
		}
	}
	return false
}