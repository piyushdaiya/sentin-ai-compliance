import json
import random
import requests
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta
from airflow import DAG
from airflow.operators.python import PythonOperator
from opensearchpy import OpenSearch

# Config
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
    
    # 1. Parse XML (Namespace Agnostic)
    tree = ET.parse(file_path)
    root = tree.getroot()
    
    # 2. Connect
    client = OpenSearch(
        hosts=[{'host': OPENSEARCH_HOST, 'port': 9200}],
        http_auth=("admin", "StrongP@ssw0rd!"),
        use_ssl=True, verify_certs=False, ssl_show_warn=False
    )

    actions = []
    
    # 3. Iterate robustly
    for entry in root:
        if not entry.tag.endswith("sdnEntry"): continue
        
        # Helper to find text safely
        def get_text(elem, suffix):
            for child in elem:
                if child.tag.endswith(suffix): return child.text
            return ""

        full_name = ""
        last = get_text(entry, "lastName")
        first = get_text(entry, "firstName")
        if last: full_name = last
        if first: full_name += ", " + first
        
        if not full_name: continue

        doc = {
            "full_name": full_name,
            "full_name_phonetic": full_name,
            "name_vector": [random.random() for _ in range(384)],
            "list_source": "OFAC-SDN"
        }
        
        actions.append(json.dumps({"index": {"_index": "sanctions-v1"}}))
        actions.append(json.dumps(doc))
        
        if len(actions) >= 1000:
            client.transport.perform_request("POST", "/_bulk", body="\n".join(actions)+"\n")
            actions = []

    if actions:
        client.transport.perform_request("POST", "/_bulk", body="\n".join(actions)+"\n")
    print("✅ Indexing Complete")

with DAG('sanction_etl', default_args=default_args, schedule_interval='@daily', start_date=datetime(2023,1,1), catchup=False) as dag:
    t1 = PythonOperator(task_id='download_ofac', python_callable=download_file)
    t2 = PythonOperator(task_id='parse_and_index', python_callable=parse_and_index)
    t1 >> t2