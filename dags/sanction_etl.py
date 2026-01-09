import json
import random
import requests
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta
from airflow import DAG
from airflow.operators.python import PythonOperator
from opensearchpy import OpenSearch

OFAC_URL = "https://www.treasury.gov/ofac/downloads/sdn.xml"
OPENSEARCH_HOST = "opensearch"

default_args = { 'owner': 'compliance', 'retries': 1, 'retry_delay': timedelta(minutes=5) }

def download_file(**kwargs):
    print("Downloading OFAC List...")
    r = requests.get(OFAC_URL)
    with open("/tmp/sdn.xml", 'wb') as f:
        f.write(r.content)
    return "/tmp/sdn.xml"

def parse_and_index(**kwargs):
    ti = kwargs['ti']
    file_path = ti.xcom_pull(task_ids='download_ofac')
    tree = ET.parse(file_path)
    root = tree.getroot()
    
    client = OpenSearch(
        hosts=[{'host': OPENSEARCH_HOST, 'port': 9200}],
        http_auth=("admin", "StrongP@ssw0rd!"),
        use_ssl=True, verify_certs=False, ssl_show_warn=False
    )

    actions = []
    
    # Iterate with namespace wildcard
    for entry in root.findall(".//{*}sdnEntry"):
        # 1. Name
        last = entry.findtext("{*}lastName")
        first = entry.findtext("{*}firstName")
        full_name = f"{last}, {first}" if last and first else (last or first or "")
        
        # 2. Alt Names
        alt_names = []
        for aka in entry.findall(".//{*}aka"):
            l = aka.findtext("{*}lastName")
            f = aka.findtext("{*}firstName")
            if l: alt_names.append(f"{l}, {f}" if f else l)

        # 3. Address
        addresses = []
        for addr in entry.findall(".//{*}address"):
            parts = []
            if addr.findtext("{*}address1"): parts.append(addr.findtext("{*}address1"))
            if addr.findtext("{*}city"): parts.append(addr.findtext("{*}city"))
            if addr.findtext("{*}country"): parts.append(addr.findtext("{*}country"))
            if parts: addresses.append(", ".join(parts))

        # 4. BICs
        bics = []
        for id_entry in entry.findall(".//{*}id"):
            id_type = id_entry.findtext("{*}idType")
            id_val = id_entry.findtext("{*}idNumber")
            if id_type and "BIC" in id_type and id_val:
                bics.append(id_val.replace(" ", ""))

        if not full_name: continue

        doc = {
            "full_name": full_name,
            "alt_names": alt_names,
            "address": " ".join(addresses),
            "bics": bics,
            "name_vector": [random.random() for _ in range(384)]
        }
        
        actions.append(json.dumps({"index": {"_index": "sanctions-v1"}}))
        actions.append(json.dumps(doc))
        
        if len(actions) >= 500:
            client.transport.perform_request("POST", "/_bulk", body="\n".join(actions)+"\n")
            actions = []

    if actions:
        client.transport.perform_request("POST", "/_bulk", body="\n".join(actions)+"\n")

with DAG('sanction_etl', default_args=default_args, schedule_interval='@daily', start_date=datetime(2023,1,1), catchup=False) as dag:
    t1 = PythonOperator(task_id='download_ofac', python_callable=download_file)
    t2 = PythonOperator(task_id='parse_and_index', python_callable=parse_and_index)
    t1 >> t2