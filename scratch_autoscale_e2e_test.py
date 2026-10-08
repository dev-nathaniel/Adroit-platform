import urllib.request
import urllib.error
import json
import time
import sys

BASE_URL = "http://localhost:8080"
GATEWAY_HEALTH = f"{BASE_URL}/healthz"
RUNTIME_URL = "http://localhost:8090"
NOMAD_URL = "http://localhost:4646"
PROMETHEUS_URL = "http://localhost:9090"
AUTOSCALER_URL = "http://localhost:8095"

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")

class Color:
    GREEN = "\033[92m"
    RED = "\033[91m"
    YELLOW = "\033[93m"
    BLUE = "\033[94m"
    CYAN = "\033[96m"
    BOLD = "\033[1m"
    RESET = "\033[0m"

def log_step(msg):
    print(f"\n{Color.BOLD}{Color.CYAN}==> {msg}{Color.RESET}")

def log_substep(msg):
    print(f"    {Color.YELLOW}* {msg}{Color.RESET}")

def log_success(msg):
    print(f"    {Color.GREEN}[OK] {msg}{Color.RESET}")

def log_error(msg):
    print(f"    {Color.RED}[FAIL] {msg}{Color.RESET}")

def api_request(method, path, data=None, token=None, base_url=BASE_URL):
    url = f"{base_url}{path}"
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    
    encoded_data = json.dumps(data).encode("utf-8") if data is not None else None
    req = urllib.request.Request(url, data=encoded_data, headers=headers, method=method)
    
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = resp.read().decode("utf-8")
            return resp.status, json.loads(body) if body else {}
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8")
        try:
            return e.code, json.loads(body)
        except Exception:
            return e.code, {"raw_error": body}
    except Exception as e:
        return 0, {"exception": str(e)}

def run_e2e_test():
    print(f"{Color.BOLD}{Color.CYAN}==========================================================================")
    print(f"     ADROIT PLATFORM — TRUE AUTO-SCALING END-TO-END VERIFICATION")
    print(f"=========================================================================={Color.RESET}")

    # 1. Health Checks across all infrastructure layers
    log_step("1. Verifying Infrastructure & Daemon Health Checks")

    # API Gateway
    log_substep("Checking API Gateway (http://localhost:8080/healthz)...")
    status, res = api_request("GET", "/healthz")
    if status == 200 and res.get("data", {}).get("status") == "healthy":
        log_success("API Gateway is online & healthy")
    else:
        log_error(f"API Gateway check failed: {res}")
        return False

    # Nomad Server
    log_substep("Checking Nomad Scheduler (http://localhost:4646/v1/status/leader)...")
    try:
        req = urllib.request.Request(f"{NOMAD_URL}/v1/status/leader")
        with urllib.request.urlopen(req, timeout=3) as resp:
            leader = json.loads(resp.read().decode())
            log_success(f"Nomad cluster leader confirmed: {leader}")
    except Exception as e:
        log_error(f"Nomad cluster leader check failed: {e}")
        return False

    # Prometheus
    log_substep("Checking Prometheus Metrics Server (http://localhost:9090/-/healthy)...")
    try:
        req = urllib.request.Request(f"{PROMETHEUS_URL}/-/healthy")
        with urllib.request.urlopen(req, timeout=3) as resp:
            log_success("Prometheus metrics server is healthy & scraping")
    except Exception as e:
        log_error(f"Prometheus check failed: {e}")
        return False

    # Nomad Autoscaler Daemon
    log_substep("Checking Nomad Autoscaler Daemon (http://localhost:8095/v1/health)...")
    try:
        req = urllib.request.Request(f"{AUTOSCALER_URL}/v1/health")
        with urllib.request.urlopen(req, timeout=3) as resp:
            log_success("Nomad Autoscaler daemon is healthy & listening on port 8095")
    except Exception as e:
        log_error(f"Nomad Autoscaler daemon check failed: {e}")
        return False

    # Runtime & Auto-Scaling Service
    log_substep("Checking Runtime Service (http://localhost:8090/healthz)...")
    status, res = api_request("GET", "/healthz", base_url=RUNTIME_URL)
    if status == 200 and res.get("data", {}).get("status") == "healthy":
        log_success("Runtime Service is online & actively reconciling")
    else:
        log_error(f"Runtime Service check failed: {res}")
        return False

    # 2. Authentication & Tenant Setup
    log_step("2. Authenticating Tenant User & Creating Project")
    ts = int(time.time())
    email = f"autoscale_dev_{ts}@adroit.run"
    password = "SuperSecurePassword123!"

    status, auth_res = api_request("POST", "/api/v1/auth/register", {
        "email": email,
        "password": password,
        "name": f"Autoscale Engineer {ts}",
        "org_name": f"Autoscale Org {ts}"
    })
    if status not in [200, 201]:
        log_error(f"Registration failed: {auth_res}")
        return False

    token = auth_res["data"]["token"]
    org_id = auth_res["data"]["organization"]["id"]
    log_success(f"Registered tenant user ({email}), Org: {org_id[:8]}")

    # Create Project
    status, proj_res = api_request("POST", "/api/v1/projects", {
        "organization_id": org_id,
        "name": f"Autoscale Demo {ts}",
        "slug": f"autoscale-{ts}"
    }, token=token)
    if status not in [200, 201]:
        log_error(f"Project creation failed: {proj_res}")
        return False
    project_id = proj_res["data"]["id"]
    log_success(f"Created project: {proj_res['data']['name']} (ID: {project_id[:8]})")

    # 3. Create Service with Auto-Scaling Enabled
    log_step("3. Provisioning Service with Elastic Auto-Scaling Policy")
    svc_slug = f"echo-scale-{ts}"
    status, svc_res = api_request("POST", "/api/v1/services", {
        "organization_id": org_id,
        "project_id": project_id,
        "name": "Elastic Echo Web",
        "slug": svc_slug,
        "type": "web",
        "source_type": "image",
        "image_url": "hashicorp/http-echo:latest",
        "port": 5678,
        "min_replicas": 1,
        "max_replicas": 4,
        "cpu_cores": 0.5,
        "memory_mb": 256
    }, token=token)
    if status not in [200, 201]:
        log_error(f"Service creation failed: {svc_res}")
        return False
    service_id = svc_res["data"]["id"]
    log_success(f"Created service: {svc_res['data']['name']} (ID: {service_id[:8]})")

    # Configure full autoscaling policy via Runtime Service
    log_substep("Configuring auto-scaling policy via PUT /api/v1/services/{id}/autoscale...")
    status, policy_res = api_request("PUT", f"/api/v1/services/{service_id}/autoscale", {
        "auto_scaling_enabled": True,
        "min_replicas": 1,
        "max_replicas": 4,
        "target_cpu_percent": 70,
        "target_memory_percent": 80,
        "target_rps": 500,
        "scale_up_cooldown_seconds": 2,
        "scale_down_cooldown_seconds": 2
    }, token=token, base_url=RUNTIME_URL)

    if status != 200 or not policy_res.get("data", {}).get("auto_scaling_enabled"):
        log_error(f"Autoscale policy configuration failed: {policy_res}")
        return False

    pol_data = policy_res["data"]
    log_success(f"Autoscale policy configured: Bounds=[{pol_data.get('min_replicas')}, {pol_data.get('max_replicas')}], TargetCPU={pol_data.get('target_cpu_percent')}%, TargetRPS={pol_data.get('target_rps')}")

    # 4. Trigger Deployment & Verify Nomad HCL 'scaling' Stanza
    log_step("4. Triggering Service Deployment & Validating Nomad HCL 'scaling' Stanza")
    status, dep_res = api_request("POST", f"/api/v1/services/{service_id}/deploy", {
        "replicas": 1,
        "strategy": "rolling"
    }, token=token)
    if status not in [200, 201, 202]:
        log_error(f"Deployment trigger failed: {dep_res}")
        return False
    deployment_id = dep_res.get("data", {}).get("id") or dep_res.get("data", {}).get("deployment", {}).get("id")
    log_success(f"Deployment initiated: {deployment_id[:8]}")

    # Wait for deployment status to become active
    log_substep("Waiting for Nomad scheduler rollout...")
    for _ in range(30):
        time.sleep(1)
        status, dep_stat = api_request("GET", f"/api/v1/deployments/{deployment_id}", token=token)
        dep_obj = dep_stat.get("data", {})
        if isinstance(dep_obj, dict) and "deployment" in dep_obj:
            dep_obj = dep_obj["deployment"]
        if status == 200 and dep_obj.get("status") in ["active", "healthy"]:
            log_success(f"Deployment reached ACTIVE status! (URL: {dep_obj.get('active_url')})")
            break
    else:
        log_error(f"Deployment timed out waiting for active status: {dep_stat}")
        return False

    # Inspect generated Nomad HCL Job Specification
    log_substep("Inspecting compiled Nomad HCL specification via GET /api/v1/deployments/{id}/spec...")
    status, spec_res = api_request("GET", f"/api/v1/deployments/{deployment_id}/spec", token=token)
    if status != 200:
        log_error(f"Failed to fetch job spec: {spec_res}")
        return False
    hcl_spec = spec_res.get("data", {}).get("job_spec", "")
    
    # Assert scaling stanza exists with authentic policy block
    if "scaling {" in hcl_spec and "min     = 1" in hcl_spec and "max     = 4" in hcl_spec and 'check "cpu_utilization"' in hcl_spec:
        log_success("VERIFIED: Nomad HCL job specification contains authentic 'scaling { min = 1, max = 4, policy { ... } }' stanza!")
    else:
        log_error(f"Missing scaling stanza in job spec:\n{hcl_spec}")
        return False

    # Wait for initial allocation to become healthy
    nomad_job_id = f"adroit-svc-{service_id}"
    log_substep(f"Waiting for live Nomad allocation of {nomad_job_id}...")
    for _ in range(25):
        time.sleep(1)
        try:
            req = urllib.request.Request(f"{NOMAD_URL}/v1/job/{nomad_job_id}/allocations")
            with urllib.request.urlopen(req, timeout=3) as resp:
                allocs = json.loads(resp.read().decode())
                running = [a for a in allocs if a.get("ClientStatus") == "running" and a.get("DesiredStatus") == "run"]
                if len(running) >= 1:
                    log_success(f"Nomad workload active: {len(running)} allocation running (ID: {running[0]['ID'][:8]})")
                    break
        except Exception:
            pass
    else:
        log_error("Timed out waiting for initial Nomad allocation")
        return False

    # 5. Live Workload Telemetry Endpoint
    log_step("5. Sampling Live Workload Telemetry via Runtime Service")
    status, met_res = api_request("GET", f"/api/v1/services/{service_id}/metrics", token=token, base_url=RUNTIME_URL)
    if status != 200:
        log_error(f"Failed to fetch live metrics: {met_res}")
        return False
    metrics = met_res.get("data", {}).get("metrics", {})
    log_success(f"Live telemetry sampled: CPU={metrics.get('cpu_percent')}%, Memory={metrics.get('memory_usage_mb'):.1f}MB/{metrics.get('memory_limit_mb'):.0f}MB ({metrics.get('memory_percent'):.1f}%), RPS={metrics.get('requests_per_sec')}, ActiveAllocs={metrics.get('active_allocations')}")

    # 6. Auto-Scaling Scale-Up Trigger (Simulated 95% CPU Spike)
    log_step("6. Testing Dynamic Elastic Scale-Up Under Heavy CPU Load")
    log_substep("Injecting 95.0% CPU spike (target is 70%)...")
    status, sim_res = api_request("POST", f"/api/v1/services/{service_id}/metrics/simulate", {
        "cpu_percent": 95.0,
        "memory_percent": 40.0,
        "rps": 60.0
    }, token=token, base_url=RUNTIME_URL)

    if status != 200:
        log_error(f"Simulate metrics failed: {sim_res}")
        return False

    decision = sim_res.get("data", {}).get("decision", {})
    if not decision.get("ShouldScale") or decision.get("DesiredReplicas") != 2:
        log_error(f"Unexpected scaling decision: {decision}")
        return False
    log_success(f"Reconciliation engine evaluated: ShouldScale=True, DesiredReplicas={decision.get('DesiredReplicas')}, Trigger='{decision.get('TriggerMetric')}' ({decision.get('Reason')})")

    # Verify live Nomad cluster allocations scale to 2
    log_substep("Verifying Nomad cluster scales up allocation count to 2...")
    for _ in range(20):
        time.sleep(1)
        try:
            req = urllib.request.Request(f"{NOMAD_URL}/v1/job/{nomad_job_id}/allocations")
            with urllib.request.urlopen(req, timeout=3) as resp:
                allocs = json.loads(resp.read().decode())
                desired_run = [a for a in allocs if a.get("DesiredStatus") == "run"]
                if len(desired_run) == 2:
                    log_success(f"Nomad cluster confirmed 2 allocations with DesiredStatus='run'!")
                    break
        except Exception:
            pass
    else:
        log_error("Nomad cluster failed to scale allocations to 2 within timeout")
        return False

    # Verify database current_replicas
    status, pol_check = api_request("GET", f"/api/v1/services/{service_id}/autoscale", token=token, base_url=RUNTIME_URL)
    if status == 200 and pol_check.get("data", {}).get("current_replicas") == 2:
        log_success("PostgreSQL metadata confirmed current_replicas = 2")
    else:
        log_error(f"Database current_replicas mismatch: {pol_check}")
        return False

    # Verify scaling event audit log
    log_substep("Checking scaling event audit log via GET /api/v1/services/{id}/scaling-events...")
    status, ev_res = api_request("GET", f"/api/v1/services/{service_id}/scaling-events", token=token, base_url=RUNTIME_URL)
    events = ev_res.get("data", {}).get("events", [])
    if len(events) > 0 and events[0].get("desired_replicas") == 2 and events[0].get("status") == "applied":
        log_success(f"Audit log recorded scale-up event (ID: {events[0]['id'][:8]}, From {events[0]['previous_replicas']} -> {events[0]['desired_replicas']}, Trigger: {events[0]['trigger_metric']})")
    else:
        log_error(f"Audit log verification failed: {ev_res}")
        return False

    # 7. Auto-Scaling Scale-Down Trigger (Traffic / CPU Drop)
    log_step("7. Testing Dynamic Elastic Scale-Down After Load Drop")
    time.sleep(3) # allow 2s cooldown window to elapse
    log_substep("Injecting low utilization score (CPU 15.0%)...")
    status, sim_down = api_request("POST", f"/api/v1/services/{service_id}/metrics/simulate", {
        "cpu_percent": 15.0,
        "memory_percent": 20.0,
        "rps": 5.0
    }, token=token, base_url=RUNTIME_URL)

    decision_down = sim_down.get("data", {}).get("decision", {})
    if not decision_down.get("ShouldScale") or decision_down.get("DesiredReplicas") != 1:
        log_error(f"Unexpected scale down decision: {decision_down}")
        return False
    log_success(f"Reconciliation engine evaluated: ShouldScale=True, DesiredReplicas=1 ({decision_down.get('Reason')})")

    # Verify Nomad cluster scales down to 1
    log_substep("Verifying Nomad cluster scales down allocation count to 1...")
    for _ in range(20):
        time.sleep(1)
        try:
            req = urllib.request.Request(f"{NOMAD_URL}/v1/job/{nomad_job_id}/allocations")
            with urllib.request.urlopen(req, timeout=3) as resp:
                allocs = json.loads(resp.read().decode())
                desired_run = [a for a in allocs if a.get("DesiredStatus") == "run"]
                if len(desired_run) == 1:
                    log_success(f"Nomad cluster confirmed 1 allocation with DesiredStatus='run'!")
                    break
        except Exception:
            pass
    else:
        log_error("Nomad cluster failed to scale allocations down to 1")
        return False

    # 8. Historical Time-Series Metrics
    log_step("8. Verifying Metrics History Time-Series API")
    status, hist_res = api_request("GET", f"/api/v1/services/{service_id}/metrics/history?limit=10", token=token, base_url=RUNTIME_URL)
    samples = hist_res.get("data", {}).get("samples", [])
    if len(samples) > 0:
        log_success(f"Retrieved {len(samples)} time-series metric samples for dashboard telemetry rendering")
    else:
        log_error(f"Metrics history empty: {hist_res}")
        return False

    # 9. Manual Scale with Elastic Range Support
    log_step("9. Verifying Variable Elastic Range Configuration in ScaleService")
    log_substep("Scaling service to 3 replicas with min_replicas=2, max_replicas=5...")
    status, scale_res = api_request("POST", f"/api/v1/services/{service_id}/scale", {
        "replicas": 3,
        "min_replicas": 2,
        "max_replicas": 5,
        "auto_scaling_enabled": True
    }, token=token)

    if status != 200 or scale_res.get("data", {}).get("replicas") != 3:
        log_error(f"Scale request failed: {scale_res}")
        return False
    log_success("Scale request applied with variable elasticity range!")

    # Verify database updated
    status, svc_final = api_request("GET", f"/api/v1/services/{service_id}", token=token)
    svc_data = svc_final.get("data", {})
    if svc_data.get("min_replicas") == 2 and svc_data.get("max_replicas") == 5:
        log_success(f"Database confirmed variable range: min_replicas={svc_data.get('min_replicas')}, max_replicas={svc_data.get('max_replicas')}")
    else:
        log_error(f"Service metadata range mismatch: {svc_final}")
        return False

    # 10. Cleanup Transient Test Job
    log_step("10. Cleaning Up Transient Test Workload on Nomad")
    try:
        del_req = urllib.request.Request(f"{NOMAD_URL}/v1/job/{nomad_job_id}?purge=true", method="DELETE")
        urllib.request.urlopen(del_req, timeout=3)
        log_success(f"Purged test Nomad job {nomad_job_id}")
    except Exception as e:
        log_substep(f"Cleanup notice: {e}")

    print(f"\n{Color.BOLD}{Color.GREEN}==========================================================================")
    print(f"     ALL TRUE AUTO-SCALING E2E TESTS PASSED SUCCESSFULLY! (100% OPERATIONAL)")
    print(f"=========================================================================={Color.RESET}\n")
    return True

if __name__ == "__main__":
    success = run_e2e_test()
    sys.exit(0 if success else 1)
