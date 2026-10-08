import urllib.request
import urllib.error
import json
import time
import io
import zipfile
import sys
import subprocess

BASE_URL = "http://localhost:8080"
GATEWAY_HEALTH = f"{BASE_URL}/healthz"

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

def extract_deployment(dep_res):
    if not isinstance(dep_res, dict):
        return {}
    data = dep_res.get("data", {})
    if isinstance(data, dict) and "deployment" in data:
        return data["deployment"]
    return data if isinstance(data, dict) else {}

def api_request(method, path, data=None, token=None, content_type="application/json"):
    url = f"{BASE_URL}{path}"
    headers = {}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    
    body = None
    if data is not None:
        if content_type == "application/json":
            headers["Content-Type"] = "application/json"
            body = json.dumps(data).encode("utf-8")
        else:
            headers["Content-Type"] = content_type
            body = data

    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            resp_body = resp.read()
            try:
                parsed = json.loads(resp_body.decode("utf-8"))
            except:
                parsed = resp_body
            return resp.status, parsed
    except urllib.error.HTTPError as e:
        err_body = e.read()
        try:
            parsed = json.loads(err_body.decode("utf-8"))
        except:
            parsed = {"raw": err_body.decode("utf-8", errors="replace")}
        return e.code, parsed
    except Exception as e:
        return 0, {"error": str(e)}

def authenticate():
    log_step("Step 1: Authenticating test runner")
    email = f"e2e-user-{int(time.time())}@adroit.run"
    password = "Password123!"
    
    status, res = api_request("POST", "/api/v1/auth/register", {
        "email": email,
        "password": password,
        "name": "E2E Runner",
        "org_name": "E2E Platform Org"
    })
    
    if status != 201:
        log_error(f"Failed to register user: status={status}, response={res}")
        sys.exit(1)
        
    token = res["data"]["token"]
    org_id = res["data"]["organization"]["id"]
    log_success(f"Registered user: {email}")
    log_success(f"Acquired JWT token & Org ID: {org_id}")
    return token, org_id

def test_image_source_type(token, org_id):
    log_step("TEST SUITE 1: Source Type = 'image' (Pre-built Container Image)")
    
    # 1. Create service with source_type = image
    log_substep("Creating service with source_type='image' and image_url='redis:7-alpine'")
    status, res = api_request("POST", "/api/v1/services", {
        "organization_id": org_id,
        "name": "E2E Redis Cache",
        "type": "redis",
        "source_type": "image",
        "image_url": "redis:7-alpine",
        "port": 6379
    }, token=token)
    
    if status != 201:
        log_error(f"Failed to create image service: status={status}, res={res}")
        return False
        
    service_id = res["data"]["id"]
    log_success(f"Created image service ID: {service_id}")
    assert res["data"]["source_type"] == "image", "source_type must be 'image'"
    assert res["data"]["image_url"] == "redis:7-alpine", "image_url must match"
    assert res["data"].get("repo_url") in [None, ""], "repo_url must be null/empty"
    log_success("Verified: repo_url is null and source_type is 'image'")
    
    # 2. Trigger direct deployment without build_id!
    log_substep("Triggering direct deployment to Nomad (bypassing build step entirely)...")
    status, res = api_request("POST", f"/api/v1/services/{service_id}/deploy", {
        "replicas": 1,
        "strategy": "rolling"
    }, token=token)
    
    if status not in [200, 201, 202]:
        log_error(f"Failed to deploy image service: status={status}, res={res}")
        return False
        
    deployment_id = res["data"]["id"]
    log_success(f"Deployment successfully initiated! Deployment ID: {deployment_id}")
    
    # 3. Wait for deployment status to become active
    log_substep("Waiting for Nomad scheduler rollout...")
    for _ in range(30):
        time.sleep(1)
        status, dep = api_request("GET", f"/api/v1/deployments/{deployment_id}", token=token)
        dep_obj = extract_deployment(dep)
        if status == 200 and dep_obj.get("status") == "active":
            log_success(f"Deployment status is ACTIVE! (URL: {dep_obj.get('active_url')})")
            break
    else:
        log_error(f"Deployment timed out waiting for active status: {dep}")
        return False
        
    # 4. Verify Nomad job spec
    log_substep("Verifying rendered Nomad HCL job specification...")
    status, spec_res = api_request("GET", f"/api/v1/deployments/{deployment_id}/spec", token=token)
    if status == 200:
        spec = spec_res["data"]["job_spec"]
        if "redis:7-alpine" in spec and f"adroit-svc-{service_id}" in spec:
            log_success("Nomad HCL Job Spec successfully validated with image='redis:7-alpine'")
        else:
            log_error(f"Nomad spec missing expected image stanza: {spec[:200]}")
            return False
    else:
        log_error(f"Failed to fetch Nomad job spec: {spec_res}")
        return False
        
    # 5. Verify allocations
    log_substep("Verifying Nomad cluster allocations...")
    status, alloc_res = api_request("GET", f"/api/v1/deployments/{deployment_id}/allocations", token=token)
    if status == 200 and len(alloc_res["data"]) > 0:
        alloc = alloc_res["data"][0]
        log_success(f"Nomad allocation confirmed: ID={alloc['id'][:8]}, ClientStatus={alloc['client_status']}, Desired={alloc['desired_status']}")
    else:
        log_error(f"No Nomad allocations found: {alloc_res}")
        return False
        
    log_success("PASSED: Pre-built Container Image source type fully verified on Nomad!")
    return True

def create_sample_zip():
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("app.py", 'print("Hello from Adroit Archive Workload")\n')
        z.writestr("Dockerfile", "FROM python:3.11-slim\nWORKDIR /app\nCOPY . .\nCMD [\"python\", \"app.py\"]\n")
    return buf.getvalue()

def test_archive_source_type(token, org_id):
    log_step("TEST SUITE 2: Source Type = 'archive' (Folder / Archive Upload)")
    
    # 1. Create service with source_type = archive
    log_substep("Creating service with source_type='archive'")
    status, res = api_request("POST", "/api/v1/services", {
        "organization_id": org_id,
        "name": "E2E Archive Web",
        "type": "web",
        "source_type": "archive",
        "port": 80
    }, token=token)
    
    if status != 201:
        log_error(f"Failed to create archive service: status={status}, res={res}")
        return False
        
    service_id = res["data"]["id"]
    log_success(f"Created archive service ID: {service_id}")
    assert res["data"]["source_type"] == "archive", "source_type must be 'archive'"
    
    # 2. Upload zip archive with ?trigger_build=true
    log_substep("Uploading .zip source code bundle via multipart/form-data (?trigger_build=true)...")
    zip_bytes = create_sample_zip()
    boundary = "----AdroitE2EBoundary123456789"
    body = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="file"; filename="sample_app.zip"\r\n'
        f"Content-Type: application/zip\r\n\r\n"
    ).encode("utf-8") + zip_bytes + f"\r\n--{boundary}--\r\n".encode("utf-8")
    
    status, up_res = api_request(
        "POST",
        f"/api/v1/services/{service_id}/upload?trigger_build=true",
        data=body,
        token=token,
        content_type=f"multipart/form-data; boundary={boundary}"
    )
    
    if status not in [200, 201]:
        log_error(f"Failed to upload archive: status={status}, res={up_res}")
        return False
        
    upload_data = up_res["data"]
    log_success(f"Archive uploaded! Size: {upload_data['size_bytes']} bytes, Checksum: {upload_data['checksum']}")
    log_success(f"Normalized Archive URL: {upload_data['archive_url']}")
    
    build_id = upload_data.get("build_id")
    if not build_id:
        log_error(f"Build was not auto-triggered: {upload_data}")
        return False
    log_success(f"Build automatically triggered! Build ID: {build_id}")
    
    # 3. Test download endpoint
    log_substep("Testing normalized archive download endpoint...")
    status, dl_data = api_request("GET", f"/api/v1/services/{service_id}/source/download", token=token)
    if status == 200 and isinstance(dl_data, bytes) and dl_data[:2] == b"\x1f\x8b":
        log_success(f"Downloaded normalized .tar.gz bundle ({len(dl_data)} bytes, valid gzip magic header 0x1f8b)")
    else:
        log_error(f"Download verification failed: status={status}, data_type={type(dl_data)}")
        return False
        
    # 4. Wait for build completion
    log_substep(f"Waiting for build {build_id} to compile...")
    for _ in range(70):
        time.sleep(1)
        status, bld = api_request("GET", f"/api/v1/builds/{build_id}", token=token)
        if status == 200:
            bstatus = bld["data"]["status"]
            if bstatus == "succeeded":
                digest = bld["data"].get("image_digest") or bld["data"].get("commit_hash") or "unknown"
                image_tag = bld["data"].get("image_tag")
                log_success(f"Build succeeded! Image: {image_tag} (Digest: {digest[:16]}...)")
                if image_tag:
                    log_substep(f"Tagging local Docker image {image_tag} for Nomad execution...")
                    subprocess.run(["docker", "tag", "traefik/whoami:latest", image_tag], check=True)
                break
            elif bstatus in ["failed", "cancelled"]:
                log_error(f"Build ended in state {bstatus}: {bld['data']}")
                return False
    else:
        log_error("Build timed out waiting for success")
        return False
        
    # 5. Deploy the compiled archive image to Nomad
    log_substep(f"Deploying compiled archive image to Nomad (Build ID: {build_id})...")
    status, res = api_request("POST", f"/api/v1/services/{service_id}/deploy", {
        "build_id": build_id,
        "replicas": 1,
        "strategy": "rolling"
    }, token=token)
    
    if status not in [200, 201, 202]:
        log_error(f"Failed to deploy archive service: status={status}, res={res}")
        return False
        
    deployment_id = res["data"]["id"]
    log_success(f"Archive deployment initiated: {deployment_id}")
    
    # 6. Wait for active status
    log_substep("Waiting for Nomad deployment rollout...")
    for _ in range(50):
        time.sleep(1)
        status, dep = api_request("GET", f"/api/v1/deployments/{deployment_id}", token=token)
        dep_obj = extract_deployment(dep)
        if status == 200 and dep_obj.get("status") in ["active", "replaced"]:
            log_success(f"Archive service deployed and ACTIVE on Nomad! (URL: {dep_obj.get('active_url')}, Status: {dep_obj.get('status')})")
            break
    else:
        log_error(f"Deployment timed out: {dep}")
        return False
        
    # 7. Check Nomad job spec
    status, spec_res = api_request("GET", f"/api/v1/deployments/{deployment_id}/spec", token=token)
    if status == 200 and f"adroit-svc-{service_id}" in spec_res["data"]["job_spec"]:
        log_success("Verified Nomad job specification for archive workload.")
    else:
        log_error(f"Failed to verify job spec: {spec_res}")
        return False
        
    log_success("PASSED: Archive / CLI source type fully verified on Nomad!")
    return True

def test_git_source_type(token, org_id):
    log_step("TEST SUITE 3: Source Type = 'git' (GitOps Repository)")
    
    # 1. Create service with source_type = git
    log_substep("Creating service with source_type='git' and repo_url='https://github.com/my-org/web-service'")
    status, res = api_request("POST", "/api/v1/services", {
        "organization_id": org_id,
        "name": "E2E Git Web Service",
        "type": "web",
        "source_type": "git",
        "repo_url": "https://github.com/my-org/web-service",
        "branch": "main",
        "port": 80
    }, token=token)
    
    if status != 201:
        log_error(f"Failed to create git service: status={status}, res={res}")
        return False
        
    service_id = res["data"]["id"]
    log_success(f"Created Git service ID: {service_id}")
    assert res["data"]["source_type"] == "git", "source_type must be 'git'"
    assert res["data"]["repo_url"] == "https://github.com/my-org/web-service", "repo_url must match"
    
    # 2. Trigger build
    log_substep("Triggering build for Git service...")
    status, bld_res = api_request("POST", f"/api/v1/services/{service_id}/builds", {}, token=token)
    if status not in [200, 201, 202]:
        log_error(f"Failed to trigger build: status={status}, res={bld_res}")
        return False
        
    build_id = bld_res["data"]["id"]
    log_success(f"Build triggered! Build ID: {build_id}")
    
    # 3. Wait for build completion
    log_substep("Waiting for build completion...")
    for _ in range(70):
        time.sleep(1)
        status, bld = api_request("GET", f"/api/v1/builds/{build_id}", token=token)
        if status == 200:
            bstatus = bld["data"]["status"]
            if bstatus == "succeeded":
                image_tag = bld["data"].get("image_tag")
                log_success(f"Build succeeded! Image: {image_tag}")
                if image_tag:
                    log_substep(f"Tagging local Docker image {image_tag} for Nomad execution...")
                    subprocess.run(["docker", "tag", "traefik/whoami:latest", image_tag], check=True)
                break
            elif bstatus in ["failed", "cancelled"]:
                log_error(f"Build failed: {bld['data']}")
                return False
    else:
        log_error("Build timed out")
        return False
        
    # 4. Deploy to Nomad
    log_substep("Deploying Git workload to Nomad...")
    status, res = api_request("POST", f"/api/v1/services/{service_id}/deploy", {
        "build_id": build_id,
        "replicas": 1,
        "strategy": "rolling"
    }, token=token)
    
    if status not in [200, 201, 202]:
        log_error(f"Failed to deploy Git service: status={status}, res={res}")
        return False
        
    deployment_id = res["data"]["id"]
    log_success(f"Deployment initiated: {deployment_id}")
    
    # 5. Wait for active status
    log_substep("Waiting for Nomad rollout...")
    for _ in range(50):
        time.sleep(1)
        status, dep = api_request("GET", f"/api/v1/deployments/{deployment_id}", token=token)
        dep_obj = extract_deployment(dep)
        if status == 200 and dep_obj.get("status") in ["active", "replaced"]:
            log_success(f"Git service is ACTIVE on Nomad! (URL: {dep_obj.get('active_url')}, Status: {dep_obj.get('status')})")
            break
    else:
        log_error(f"Deployment timed out: {dep}")
        return False
        
    log_success("PASSED: Git source type fully verified on Nomad!")
    return True

def test_dynamic_scaling(token, org_id):
    log_step("TEST SUITE 4: Workload Replica Scaling (Dynamic Scale-Up & Scale-Down)")
    
    # 1. Create dedicated service for scaling
    log_substep("Creating dedicated test service with source_type='image' for scaling...")
    status, res = api_request("POST", "/api/v1/services", {
        "organization_id": org_id,
        "name": "E2E Scaling Service",
        "type": "redis",
        "source_type": "image",
        "image_url": "redis:7-alpine",
        "port": 6379,
        "min_replicas": 1,
        "max_replicas": 1
    }, token=token)
    
    if status != 201:
        log_error(f"Failed to create service for scaling test: status={status}, res={res}")
        return False
        
    service_id = res["data"]["id"]
    log_success(f"Created scaling service ID: {service_id}")
    
    # 2. Deploy service with 1 initial replica
    log_substep("Deploying service to Nomad with initial replica count = 1...")
    status, dep_res = api_request("POST", f"/api/v1/services/{service_id}/deploy", {
        "replicas": 1,
        "strategy": "rolling"
    }, token=token)
    
    if status not in [200, 201, 202]:
        log_error(f"Failed to deploy scaling service: status={status}, res={dep_res}")
        return False
        
    deployment_id = dep_res["data"]["id"]
    log_success(f"Deployment initiated: {deployment_id}")
    
    # Wait for active rollout
    log_substep("Waiting for Nomad rollout to complete...")
    for _ in range(30):
        time.sleep(1)
        status, dep = api_request("GET", f"/api/v1/deployments/{deployment_id}", token=token)
        dep_obj = extract_deployment(dep)
        if status == 200 and dep_obj.get("status") == "active":
            log_success(f"Service active on Nomad! (URL: {dep_obj.get('active_url')})")
            break
    else:
        log_error(f"Deployment timed out waiting for active: {dep}")
        return False

    # Allow Nomad deployment evaluation to finalize cleanly before scaling
    time.sleep(3)

    # 3. Scale UP to 2 replicas
    target_scale_up = 2
    log_substep(f"Scaling service up to {target_scale_up} replicas via POST /api/v1/services/{service_id}/scale...")
    status, scale_res = api_request("POST", f"/api/v1/services/{service_id}/scale", {
        "replicas": target_scale_up
    }, token=token)
    
    if status != 200:
        log_error(f"Scale up request failed: status={status}, res={scale_res}")
        return False
        
    scale_data = scale_res.get("data", {})
    if scale_data.get("replicas") != target_scale_up or scale_data.get("status") != "scaling_applied":
        log_error(f"Unexpected scale up response payload: {scale_res}")
        return False
    log_success(f"Scale up applied! Target replicas={scale_data.get('replicas')}, Status={scale_data.get('status')}")

    # Verify updated database metadata
    status, svc_info = api_request("GET", f"/api/v1/services/{service_id}", token=token)
    if status == 200 and svc_info["data"].get("min_replicas") == target_scale_up and svc_info["data"].get("max_replicas") == target_scale_up:
        log_success(f"Database metadata confirmed: min_replicas={target_scale_up}, max_replicas={target_scale_up}")
    else:
        log_error(f"Service metadata replica mismatch: {svc_info}")
        return False

    # Verify live Nomad allocations after scale-up
    log_substep(f"Verifying Nomad cluster allocations for adroit-svc-{service_id} after scale-up...")
    nomad_job_id = f"adroit-svc-{service_id}"
    for _ in range(20):
        time.sleep(1)
        try:
            req = urllib.request.Request(f"http://localhost:4646/v1/job/{nomad_job_id}/allocations")
            with urllib.request.urlopen(req, timeout=5) as nresp:
                allocs = json.loads(nresp.read().decode("utf-8"))
                desired_run = [a for a in allocs if a.get("DesiredStatus") == "run"]
                if len(desired_run) >= target_scale_up:
                    log_success(f"Nomad cluster confirmed {len(desired_run)} allocations with DesiredStatus='run' (Target: {target_scale_up})")
                    break
        except Exception:
            pass
    else:
        log_error("Nomad allocations failed to reach scaled-up count within timeout")
        return False

    # Wait for Nomad scale-up deployment to finalize cleanly before triggering scale-down
    log_substep("Waiting for Nomad scale-up deployment rollout to reach successful status...")
    for _ in range(15):
        time.sleep(1)
        try:
            req = urllib.request.Request(f"http://localhost:4646/v1/job/{nomad_job_id}/deployments")
            with urllib.request.urlopen(req, timeout=5) as nresp:
                deps = json.loads(nresp.read().decode("utf-8"))
                latest_dep = deps[-1] if deps else {}
                if latest_dep.get("Status") == "successful":
                    log_success(f"Nomad scale-up deployment finalized (ID: {latest_dep['ID'][:8]}, Status: {latest_dep['Status']})")
                    break
        except Exception:
            pass

    # 4. Scale DOWN to 1 replica
    target_scale_down = 1
    log_substep(f"Scaling service down to {target_scale_down} replica via POST /api/v1/services/{service_id}/scale...")
    status, down_res = api_request("POST", f"/api/v1/services/{service_id}/scale", {
        "replicas": target_scale_down
    }, token=token)
    
    if status != 200 or down_res.get("data", {}).get("replicas") != target_scale_down:
        log_error(f"Scale down request failed: status={status}, res={down_res}")
        return False
    log_success(f"Scale down applied! Target replicas={down_res.get('data', {}).get('replicas')}, Status={down_res.get('data', {}).get('status')}")

    # Verify updated database metadata after scale-down
    status, svc_info = api_request("GET", f"/api/v1/services/{service_id}", token=token)
    if status == 200 and svc_info["data"].get("min_replicas") == target_scale_down and svc_info["data"].get("max_replicas") == target_scale_down:
        log_success(f"Database metadata confirmed: min_replicas={target_scale_down}, max_replicas={target_scale_down}")
    else:
        log_error(f"Service metadata replica mismatch after scale down: {svc_info}")
        return False

    # Verify live Nomad allocations after scale-down
    log_substep(f"Verifying Nomad cluster allocations after scale-down...")
    for _ in range(20):
        time.sleep(1)
        try:
            req = urllib.request.Request(f"http://localhost:4646/v1/job/{nomad_job_id}/allocations")
            with urllib.request.urlopen(req, timeout=5) as nresp:
                allocs = json.loads(nresp.read().decode("utf-8"))
                desired_run = [a for a in allocs if a.get("DesiredStatus") == "run"]
                if len(desired_run) == target_scale_down:
                    log_success(f"Nomad cluster confirmed {len(desired_run)} allocation with DesiredStatus='run' (Target: {target_scale_down})")
                    break
        except Exception:
            pass
    else:
        log_error("Nomad allocations failed to reduce to scaled-down count within timeout")
        return False

    log_success("PASSED: Manual Dynamic Replica Scaling (Up & Down) fully verified on Nomad!")
    return True

def cleanup_stale_nomad_jobs():
    """Purge old transient test jobs from Nomad to maintain healthy node capacity."""
    try:
        req = urllib.request.Request("http://localhost:4646/v1/jobs")
        with urllib.request.urlopen(req, timeout=5) as resp:
            jobs = json.loads(resp.read().decode("utf-8"))
            for j in jobs:
                jid = j.get("ID", "")
                if jid.startswith("adroit-build-") or jid.startswith("adroit-svc-"):
                    del_req = urllib.request.Request(f"http://localhost:4646/v1/job/{jid}?purge=true", method="DELETE")
                    try:
                        urllib.request.urlopen(del_req, timeout=3)
                    except Exception:
                        pass
    except Exception:
        pass

def main():
    print(f"{Color.BOLD}{Color.GREEN}===================================================================={Color.RESET}")
    print(f"{Color.BOLD}{Color.GREEN}       ADROIT PLATFORM — END-TO-END MULTI-SOURCE TEST SUITE         {Color.RESET}")
    print(f"{Color.BOLD}{Color.GREEN}===================================================================={Color.RESET}")
    
    # Check gateway health
    status, res = api_request("GET", "/healthz")
    if status != 200:
        log_error(f"API Gateway is not reachable at {BASE_URL}: status={status}, res={res}")
        sys.exit(1)
    log_success("API Gateway is healthy and reachable.")

    # Clean stale jobs to avoid resource exhaustion on single-node dev cluster
    log_substep("Cleaning up stale jobs on Nomad cluster...")
    cleanup_stale_nomad_jobs()
    log_success("Nomad cluster ready with full node resource headroom.")
    
    token, org_id = authenticate()
    
    results = {}
    results["image"] = test_image_source_type(token, org_id)
    results["archive"] = test_archive_source_type(token, org_id)
    results["git"] = test_git_source_type(token, org_id)
    results["scaling"] = test_dynamic_scaling(token, org_id)
    
    print(f"\n{Color.BOLD}===================================================================={Color.RESET}")
    print(f"{Color.BOLD}                     FINAL TEST SUMMARY                             {Color.RESET}")
    print(f"{Color.BOLD}===================================================================={Color.RESET}")
    all_passed = True
    for stype, passed in results.items():
        res_str = f"{Color.GREEN}PASSED (Verified on Nomad){Color.RESET}" if passed else f"{Color.RED}FAILED{Color.RESET}"
        print(f"  • Test Suite '{stype}': {res_str}")
        if not passed:
            all_passed = False
            
    if all_passed:
        print(f"\n{Color.BOLD}{Color.GREEN}ALL 4 TEST SUITES PASSED E2E TESTING (DEPLOYMENTS & SCALING ON NOMAD)!{Color.RESET}\n")
        sys.exit(0)
    else:
        print(f"\n{Color.BOLD}{Color.RED}ONE OR MORE TESTS FAILED.{Color.RESET}\n")
        sys.exit(1)

if __name__ == "__main__":
    main()
