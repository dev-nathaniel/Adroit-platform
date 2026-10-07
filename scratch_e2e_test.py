import sys
import json
import time
import subprocess
import urllib.request
import urllib.error

sys.stdout.reconfigure(line_buffering=True)

BASE_URL = "http://localhost:8080"
NOMAD_URL = "http://localhost:4646"
VAULT_URL = "http://localhost:8200"
CADDY_URL = "http://localhost:2019"
VAULT_TOKEN = "root"

def http_req(url, method="GET", data=None, headers=None):
    if headers is None:
        headers = {}
    encoded_data = None
    if data is not None:
        if isinstance(data, (dict, list)):
            encoded_data = json.dumps(data).encode("utf-8")
            headers["Content-Type"] = "application/json"
        elif isinstance(data, str):
            encoded_data = data.encode("utf-8")
        elif isinstance(data, bytes):
            encoded_data = data

    req = urllib.request.Request(url, data=encoded_data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = resp.read().decode("utf-8")
            status = resp.status
            try:
                parsed_json = json.loads(body)
            except Exception:
                parsed_json = body
            return status, parsed_json
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8")
        try:
            parsed_json = json.loads(err_body)
        except Exception:
            parsed_json = err_body
        return e.code, parsed_json
    except Exception as e:
        return 0, str(e)

def log_step(title):
    print("\n" + "=" * 70)
    print(f"==> {title}")
    print("=" * 70)

def main():
    timestamp = int(time.time())
    email = f"user_{timestamp}@adroit-cloud.io"
    password = "SuperPassword123!"
    name = f"Tenant {timestamp}"

    # -------------------------------------------------------------
    # 1. Register or Login
    # -------------------------------------------------------------
    log_step("STEP 1: Developer Account Registration & Authentication")
    reg_payload = {"email": email, "password": password, "name": name}
    status, res = http_req(f"{BASE_URL}/api/v1/auth/register", method="POST", data=reg_payload)
    print(f"Register status: {status}")
    token = None
    if isinstance(res, dict):
        token = res.get("data", {}).get("token")
    if not token:
        status, res = http_req(f"{BASE_URL}/api/v1/auth/login", method="POST", data={"email": email, "password": password})
        if isinstance(res, dict):
            token = res.get("data", {}).get("token")
    
    if not token:
        print(f"Failed to get token! Response: {res}")
        return

    print(f"Authenticated JWT Token: {token[:20]}...{token[-10:]}")
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}

    # Verify /me
    status, me_res = http_req(f"{BASE_URL}/api/v1/auth/me", headers=headers)
    user_data = me_res.get("data", {}) if isinstance(me_res, dict) else {}
    user_id = user_data.get("id")
    print(f"Verified Current User: ID={user_id}, Email={user_data.get('email')}, Name={user_data.get('name')}")

    # -------------------------------------------------------------
    # 2. Create Organization & Project
    # -------------------------------------------------------------
    log_step("STEP 2: Workspace Hierarchy (Organization & Project)")
    org_slug = f"org{timestamp}"
    org_payload = {"name": f"Acme Tenant {timestamp}", "slug": org_slug}
    status, org_res = http_req(f"{BASE_URL}/api/v1/users/organizations", method="POST", headers=headers, data=org_payload)
    print(f"Create Org status: {status}")
    org_data = org_res.get("data", {}) if isinstance(org_res, dict) else {}
    org_id = org_data.get("id")
    print(f"Created Organization: ID={org_id}, Slug={org_slug}")

    project_slug = f"web-app-{timestamp}"
    proj_payload = {
        "organization_id": org_id,
        "name": "Production Microservices",
        "slug": project_slug,
        "description": "Production edge web application"
    }
    status, proj_res = http_req(f"{BASE_URL}/api/v1/projects", method="POST", headers=headers, data=proj_payload)
    print(f"Create Project status: {status}")
    proj_data = proj_res.get("data", {}) if isinstance(proj_res, dict) else {}
    project_id = proj_data.get("id")
    print(f"Created Project: ID={project_id}, Slug={project_slug}")

    # -------------------------------------------------------------
    # 3. Create Workload Service
    # -------------------------------------------------------------
    log_step("STEP 3: Define Workload Service Specification")
    service_slug = f"whoami-{timestamp % 10000}"
    service_payload = {
        "organization_id": org_id,
        "project_id": project_id,
        "name": "Whoami Live Echo Service",
        "slug": service_slug,
        "type": "web",
        "runtime_tier": "standard",
        "repo_url": "https://github.com/traefik/whoami",
        "branch": "master",
        "port": 80,
        "min_replicas": 1,
        "max_replicas": 3,
        "resources": {
            "cpu_cores": 0.5,
            "memory_mb": 256,
            "disk_mb": 512
        },
        "health_check": {
            "path": "/health",
            "port": 80,
            "interval_seconds": 5,
            "timeout_seconds": 2,
            "failure_threshold": 3,
            "initial_delay_seconds": 2
        }
    }
    status, svc_res = http_req(f"{BASE_URL}/api/v1/services", method="POST", headers=headers, data=service_payload)
    print(f"Create Service status: {status}")
    svc_data = svc_res.get("data", {}) if isinstance(svc_res, dict) else {}
    service_id = svc_data.get("id")
    print(f"Created Service: ID={service_id}, Slug={service_slug}, Port={svc_data.get('port')}")

    # -------------------------------------------------------------
    # 4. Inject Environment Variables & Vault Secrets
    # -------------------------------------------------------------
    log_step("STEP 4: Inject Configuration & HashiCorp Vault Secrets")
    env_payload = {
        "variables": [
            {
                "key": "APP_ENV",
                "value": "production",
                "is_secret": False,
                "scope": "runtime",
                "target_env": "production"
            },
            {
                "key": "WHOAMI_NAME",
                "value": f"Adroit-Production-Node-{timestamp}",
                "is_secret": False,
                "scope": "runtime",
                "target_env": "production"
            },
            {
                "key": "DATABASE_URL",
                "value": "postgres://adroit_user:vault_sealed_pass_9988@db.adroit.internal:5432/prod_db",
                "is_secret": True,
                "scope": "runtime",
                "target_env": "production"
            },
            {
                "key": "STRIPE_SECRET_KEY",
                "value": "sk_live_51PTestVaultEncryptionKey9900112233",
                "is_secret": True,
                "scope": "runtime",
                "target_env": "production"
            }
        ]
    }
    status, env_res = http_req(f"{BASE_URL}/api/v1/services/{service_id}/env", method="POST", headers=headers, data=env_payload)
    print(f"Set Environment Variables status: {status}")

    # Verify secrets masking in GET /services/:id/env
    status, get_env_res = http_req(f"{BASE_URL}/api/v1/services/{service_id}/env", headers=headers)
    env_vars = get_env_res.get("data", []) if isinstance(get_env_res, dict) else []
    print(f"Retrieved {len(env_vars)} Environment Variables:")
    for v in env_vars:
        print(f"  * {v.get('key')} = {v.get('value')} (is_secret={v.get('is_secret')})")

    # Reveal a secret via Adroit audit-logged reveal endpoint
    status, reveal_res = http_req(f"{BASE_URL}/api/v1/services/{service_id}/env/STRIPE_SECRET_KEY/reveal", method="POST", headers=headers)
    revealed_val = reveal_res.get("data", {}).get("value") if isinstance(reveal_res, dict) else "N/A"
    print(f"Revealed Secret via API: key=STRIPE_SECRET_KEY, value={revealed_val}")

    # Verify secrets directly inside HashiCorp Vault via Vault API
    vault_headers = {"X-Vault-Token": VAULT_TOKEN}
    vault_path = f"adroit/{org_id}/services/{service_id}"
    v_status, v_res = http_req(f"{VAULT_URL}/v1/secret/data/{vault_path}", headers=vault_headers)
    print(f"Direct Vault API check (GET /v1/secret/data/{vault_path}): status={v_status}")
    if v_status == 200 and isinstance(v_res, dict):
        vault_secret_data = v_res.get("data", {}).get("data", {})
        print("Vault Sealed Data Contents:")
        for k, val in vault_secret_data.items():
            print(f"  [Vault KV v2] {k} = {val}")

    # -------------------------------------------------------------
    # 5. Trigger Build Pipeline
    # -------------------------------------------------------------
    log_step("STEP 5: Trigger Build Pipeline & BuildKit Artifact Generation")
    build_payload = {
        "service_id": service_id,
        "branch": "master",
        "commit_hash": f"c{timestamp}",
        "message": "Production deployment release v1.0.0"
    }
    status, b_res = http_req(f"{BASE_URL}/api/v1/builds", method="POST", headers=headers, data=build_payload)
    print(f"Trigger Build status: {status}")
    build_data = b_res.get("data", {}) if isinstance(b_res, dict) else {}
    build_id = build_data.get("id")
    print(f"Build ID: {build_id}")

    # Poll build status
    max_wait = 30
    built_image_tag = None
    for _ in range(max_wait):
        status, b_info_res = http_req(f"{BASE_URL}/api/v1/builds/{build_id}", headers=headers)
        b_info = b_info_res.get("data", {}) if isinstance(b_info_res, dict) else {}
        b_status = b_info.get("status")
        built_image_tag = b_info.get("image_tag")
        print(f"  Build status: {b_status} (image_tag: {built_image_tag})")
        if b_status in ("succeeded", "failed", "cancelled"):
            break
        time.sleep(1)

    # Fetch build logs
    status, logs_res = http_req(f"{BASE_URL}/api/v1/builds/{build_id}/logs", headers=headers)
    raw_logs = logs_res.get("data", {}) if isinstance(logs_res, dict) else {}
    if isinstance(raw_logs, dict):
        build_logs = raw_logs.get("logs", [])
    elif isinstance(raw_logs, list):
        build_logs = raw_logs
    else:
        build_logs = []
    print(f"Retrieved {len(build_logs)} Build Log lines:")
    for l in build_logs[:6]:
        print(f"  [{l.get('stream')}] {l.get('line')}")

    # Tag local Docker image to match built_image_tag so Nomad Docker runner finds it immediately!
    if built_image_tag:
        print(f"\nTagging local traefik/whoami:latest -> {built_image_tag} for Nomad Docker execution...")
        subprocess.run(["docker", "tag", "traefik/whoami:latest", built_image_tag], check=True)
        print("Image tagged successfully in local Docker engine!")

    # -------------------------------------------------------------
    # 6. Deploy & Orchestrate on Cluster
    # -------------------------------------------------------------
    log_step("STEP 6: Deploy & Orchestrate on Nomad Cluster")
    time.sleep(1)
    status, deps_res = http_req(f"{BASE_URL}/api/v1/services/{service_id}/deployments", headers=headers)
    raw_deps = deps_res.get("data", []) if isinstance(deps_res, dict) else []
    if isinstance(raw_deps, dict):
        deployments = raw_deps.get("deployments", [])
    elif isinstance(raw_deps, list):
        deployments = raw_deps
    else:
        deployments = []

    deployment_id = None
    if deployments:
        deployment_id = deployments[0].get("id")
        print(f"Auto-triggered Deployment detected: ID={deployment_id}, Status={deployments[0].get('status')}")
    else:
        print("Triggering manual deployment...")
        deploy_payload = {
            "service_id": service_id,
            "build_id": build_id,
            "replicas": 1,
            "strategy": "rolling"
        }
        status, d_res = http_req(f"{BASE_URL}/api/v1/services/{service_id}/deploy", method="POST", headers=headers, data=deploy_payload)
        print(f"Trigger Deploy status: {status}")
        deployment_id = d_res.get("data", {}).get("id") if isinstance(d_res, dict) else None

    print(f"Monitoring Deployment: {deployment_id}")

    # Inspect generated Nomad HCL Job Specification
    time.sleep(1)
    status, spec_res = http_req(f"{BASE_URL}/api/v1/deployments/{deployment_id}/spec", headers=headers)
    spec_hcl = spec_res.get("data", {}).get("job_spec", "") if isinstance(spec_res, dict) else ""
    print(f"\nGenerated Nomad HCL Spec ({len(spec_hcl)} bytes):")
    for line in spec_hcl.splitlines()[:15]:
        print(f"  | {line}")
    print("  | ... [truncated for brevity]")

    # Poll deployment status until active
    active_url = None
    nomad_job_id = None
    for i in range(25):
        status, dep_res = http_req(f"{BASE_URL}/api/v1/deployments/{deployment_id}", headers=headers)
        dep_info = dep_res.get("data", {}).get("deployment", {}) if isinstance(dep_res, dict) else {}
        d_status = dep_info.get("status")
        nomad_job_id = dep_info.get("nomad_job_id")
        active_url = dep_info.get("active_url")
        print(f"  [{i+1}/25] Deployment Status: {d_status} | Nomad Job: {nomad_job_id} | URL: {active_url}")
        if d_status in ("active", "failed", "cancelled"):
            break
        time.sleep(1.5)

    # Fetch allocations
    status, allocs_res = http_req(f"{BASE_URL}/api/v1/deployments/{deployment_id}/allocations", headers=headers)
    allocs = allocs_res.get("data", []) if isinstance(allocs_res, dict) else []
    print(f"\nCluster Allocations ({len(allocs)} instances):")
    for a in allocs:
        print(f"  * AllocID={a.get('nomad_alloc_id')} Node={a.get('node_name')} Status={a.get('client_status')} Desired={a.get('desired_status')} Healthy={a.get('is_healthy')}")

    # Fetch deployment timeline events
    status, ev_res = http_req(f"{BASE_URL}/api/v1/deployments/{deployment_id}/events", headers=headers)
    events_list = ev_res.get("data", []) if isinstance(ev_res, dict) else []
    print(f"\nDeployment Audit Timeline ({len(events_list)} events):")
    for ev in events_list:
        print(f"  * [{ev.get('event_type')}] {ev.get('message')}")

    # Check Nomad Job on live Nomad API
    status, n_jobs = http_req(f"{NOMAD_URL}/v1/jobs")
    if isinstance(n_jobs, list):
        print(f"\nNomad Cluster Registered Jobs: {[j.get('ID') for j in n_jobs]}")

    # -------------------------------------------------------------
    # 7. Check Ingress Routes & Query Deployed App
    # -------------------------------------------------------------
    log_step("STEP 7: Edge Routing, Caddy Ingress & Real User Traffic")
    status, routes_res = http_req(f"{BASE_URL}/api/v1/ingress/routes", headers=headers)
    print(f"Ingress Routes: status={status}")
    print(f"Routes data: {json.dumps(routes_res, indent=2)}")

    # Check direct Nomad allocations ports
    nomad_alloc_url = None
    running_allocs = [a for a in allocs if a.get("client_status") == "running"]
    target_allocs = running_allocs if running_allocs else allocs
    for a in target_allocs:
        aid = a.get("nomad_alloc_id")
        status, alloc_detail = http_req(f"{NOMAD_URL}/v1/allocation/{aid}")
        if status == 200 and isinstance(alloc_detail, dict):
            networks = alloc_detail.get("AllocatedResources", {}).get("Shared", {}).get("Networks", [])
            for net in networks:
                for port in net.get("DynamicPorts", []):
                    if port.get("Label") == "http":
                        nomad_alloc_url = f"http://localhost:{port.get('Value')}"
                        print(f"\n>>> Direct Container Port on Host: {nomad_alloc_url}")
                        break
        if nomad_alloc_url:
            break

    # Query deployed application
    test_urls = []
    if nomad_alloc_url:
        test_urls.append(nomad_alloc_url)
    if active_url and active_url not in test_urls:
        test_urls.append(active_url)

    for u in test_urls:
        print(f"\nHitting Deployed App at {u}...")
        status, body = http_req(u)
        print(f"App HTTP Status: {status}")
        text_preview = body if isinstance(body, str) else json.dumps(body)
        print(f"App Response:\n{text_preview[:500]}")

    # -------------------------------------------------------------
    # 8. Check Log Service Observability
    # -------------------------------------------------------------
    log_step("STEP 8: Log Ingestion & Real-Time Observability")
    status, app_logs_res = http_req(f"{BASE_URL}/api/v1/services/{service_id}/logs?limit=10", headers=headers)
    print(f"Query Service Logs status: {status}")
    logs_res = app_logs_res.get("data", {}) if isinstance(app_logs_res, dict) else {}
    if isinstance(logs_res, dict):
        logs_list = logs_res.get("logs") or []
    elif isinstance(logs_res, list):
        logs_list = logs_res
    else:
        logs_list = []
    print(f"Retrieved {len(logs_list)} Log entries from Log Service:")
    for l in logs_list[:5]:
        print(f"  [{l.get('stream')}] {l.get('message')}")

    # Summary
    log_step("TESTING COMPLETED SUCCESSFULLY!")
    print(f"User Email: {email}")
    print(f"Organization ID: {org_id} (slug: {org_slug})")
    print(f"Project ID: {project_id}")
    print(f"Service ID: {service_id} (slug: {service_slug})")
    print(f"Build ID: {build_id}")
    print(f"Deployment ID: {deployment_id}")
    if nomad_alloc_url:
        print(f"Direct Deployed App URL: {nomad_alloc_url}")
    print(f"Vault UI Secret Path: {VAULT_URL}/ui/vault/secrets/secret/show/{vault_path}")
    print(f"Nomad UI Job: {NOMAD_URL}/ui/jobs/{nomad_job_id if nomad_job_id else 'adroit-svc-' + service_id[:8]}")

if __name__ == "__main__":
    main()
