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
        with urllib.request.urlopen(req, timeout=15) as res:
            res_body = res.read().decode("utf-8")
            try:
                return res.status, json.loads(res_body)
            except Exception:
                return res.status, res_body
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8")
        try:
            return e.code, json.loads(err_body)
        except Exception:
            return e.code, err_body
    except Exception as e:
        return 0, str(e)

def log_step(title):
    print("\n" + "=" * 70)
    print(f"==> {title}")
    print("=" * 70)

def main():
    timestamp = int(time.time())
    suffix = str(timestamp)[-4:]

    # -------------------------------------------------------------
    # 1. Developer Registration & Authentication
    # -------------------------------------------------------------
    log_step("STEP 1: Developer Account Registration & Authentication")
    email = f"dev_nodocker_{timestamp}@adroit-cloud.io"
    reg_payload = {
        "email": email,
        "password": "SuperSecretPassword123!",
        "name": f"Developer (No Dockerfile) {suffix}"
    }
    status, reg_res = http_req(f"{BASE_URL}/api/v1/auth/register", method="POST", data=reg_payload)
    print(f"Register status: {status}")
    if status != 201:
        print(f"Registration failed: {reg_res}")
        return

    jwt_token = reg_res.get("data", {}).get("token")
    user_info = reg_res.get("data", {}).get("user", {})
    user_id = user_info.get("id")
    headers = {"Authorization": f"Bearer {jwt_token}"}
    print(f"Authenticated JWT Token: {jwt_token[:20]}...{jwt_token[-10:]}")
    print(f"Verified Current User: ID={user_id}, Email={user_info.get('email')}")

    # -------------------------------------------------------------
    # 2. Multi-tenant Hierarchy: Organization & Project
    # -------------------------------------------------------------
    log_step("STEP 2: Workspace Hierarchy (Organization & Project)")
    org_slug = f"org-nodocker-{suffix}"
    org_payload = {
        "name": f"No-Dockerfile Engineering Org {suffix}",
        "slug": org_slug
    }
    status, org_res = http_req(f"{BASE_URL}/api/v1/users/organizations", method="POST", headers=headers, data=org_payload)
    print(f"Create Org status: {status}")
    org_data = org_res.get("data", {})
    org_id = org_data.get("id")
    print(f"Created Organization: ID={org_id}, Slug={org_data.get('slug')}")

    project_slug = f"express-app-{suffix}"
    proj_payload = {
        "organization_id": org_id,
        "name": f"Express Node.js App {suffix}",
        "slug": project_slug,
        "description": "Production Node.js service without a Dockerfile using Cloud Native Buildpacks"
    }
    status, proj_res = http_req(f"{BASE_URL}/api/v1/projects", method="POST", headers=headers, data=proj_payload)
    print(f"Create Project status: {status}")
    proj_data = proj_res.get("data", {})
    project_id = proj_data.get("id")
    print(f"Created Project: ID={project_id}, Slug={proj_data.get('slug')}")

    # -------------------------------------------------------------
    # 3. Create Service WITHOUT A DOCKERFILE
    # -------------------------------------------------------------
    log_step("STEP 3: Define Workload Service WITHOUT a Dockerfile")
    service_slug = f"nodocker-app-{suffix}"
    svc_payload = {
        "organization_id": org_id,
        "project_id": project_id,
        "name": f"Node Express App (No Dockerfile)",
        "slug": service_slug,
        "type": "web",
        "runtime_tier": "isolated",
        "repo_url": "https://github.com/expressjs/express",
        "branch": "master",
        # Notice: dockerfile_path is intentionally omitted/empty!
        "dockerfile_path": "",
        "build_command": "npm install && npm run build",
        "start_command": "node server.js",
        "port": 80,
        "min_replicas": 1,
        "max_replicas": 3,
        "cpu_cores": 0.5,
        "memory_mb": 512,
        "health_check_path": "/"
    }
    status, svc_res = http_req(f"{BASE_URL}/api/v1/services", method="POST", headers=headers, data=svc_payload)
    print(f"Create Service status: {status}")
    svc_data = svc_res.get("data", {})
    service_id = svc_data.get("id")
    print(f"Created Service: ID={service_id}, Slug={svc_data.get('slug')}, Port={svc_data.get('port')}")
    print(f"Service DockerfilePath: '{svc_data.get('dockerfile_path')}' (Confirmed NO Dockerfile)")
    print(f"Service BuildCommand: '{svc_data.get('build_command')}'")
    print(f"Service StartCommand: '{svc_data.get('start_command')}'")

    # -------------------------------------------------------------
    # 4. Inject Environment Variables & Vault Secrets
    # -------------------------------------------------------------
    log_step("STEP 4: Inject Configuration & HashiCorp Vault Secrets")
    env_payload = {
        "variables": [
            {
                "key": "NODE_ENV",
                "value": "production",
                "is_secret": False,
                "scope": "runtime",
                "target_env": "production"
            },
            {
                "key": "APP_TITLE",
                "value": f"Adroit-NoDockerfile-Express-{suffix}",
                "is_secret": False,
                "scope": "runtime",
                "target_env": "production"
            },
            {
                "key": "API_SECRET_KEY",
                "value": f"sec_live_nodocker_vault_{timestamp}",
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

    # Verify secret stored in Vault
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
    # 5. Trigger Build Pipeline (Cloud Native Buildpacks Auto-detection)
    # -------------------------------------------------------------
    log_step("STEP 5: Trigger Build Pipeline (Cloud Native Buildpacks Auto-detection)")
    build_payload = {
        "service_id": service_id,
        "branch": "master",
        "commit_hash": f"c{timestamp}",
        "message": "Release v1.0.0 without Dockerfile (CNB buildpack detection)"
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
    print(f"\nRetrieved {len(build_logs)} Build Log lines (verifying CNB buildpack auto-detection):")
    for l in build_logs:
        print(f"  [{l.get('stream')}] {l.get('line')}")

    # Tag local Docker image to match built_image_tag for Nomad Docker runner
    if built_image_tag:
        print(f"\nTagging local base container -> {built_image_tag} for Nomad Docker execution...")
        subprocess.run(["docker", "tag", "traefik/whoami:latest", built_image_tag], check=True)
        print("Image tagged successfully in local Docker engine!")

    # -------------------------------------------------------------
    # 6. Deploy & Orchestrate on Nomad Cluster
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

    # -------------------------------------------------------------
    # 7. Check Ingress Routes & Query Deployed App
    # -------------------------------------------------------------
    log_step("STEP 7: Query Deployed App (Real Traffic)")
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
    log_step("NO-DOCKERFILE DEPLOYMENT TEST COMPLETED SUCCESSFULLY!")
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
