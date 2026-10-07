import urllib.request
import json

base_url = "http://localhost:8080"

def make_req(path, data=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    body = json.dumps(data).encode("utf-8") if data else None
    req = urllib.request.Request(f"{base_url}{path}", data=body, headers=headers)
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8")
        try:
            return e.code, json.loads(err_body)
        except:
            return e.code, {"error": err_body}

# Try login
status, res = make_req("/api/v1/auth/login", {"email": "e2e-tester@adroit.run", "password": "Password123!"})
print("Login status:", status, res)

if status != 200:
    # Register
    status, res = make_req("/api/v1/auth/register", {
        "email": "e2e-tester@adroit.run",
        "password": "Password123!",
        "name": "E2E Tester",
        "org_name": "E2E Org"
    })
    print("Register status:", status, res)
