import urllib.request
import json
import time

job_hcl = """job "test-artifact" {
  datacenters = ["dc1"]
  type = "batch"
  group "test" {
    task "getter" {
      driver = "docker"
      artifact {
        source = "http://host.docker.internal:8084/healthz"
        destination = "local/src"
      }
      config {
        image = "busybox:latest"
        command = "ls"
        args = ["-la", "local/src"]
      }
    }
  }
}
"""

req = urllib.request.Request("http://localhost:4646/v1/jobs/parse", data=json.dumps({"JobHCL": job_hcl}).encode(), headers={"Content-Type": "application/json"})
parsed = json.loads(urllib.request.urlopen(req).read())
req2 = urllib.request.Request("http://localhost:4646/v1/jobs", data=json.dumps({"Job": parsed}).encode(), headers={"Content-Type": "application/json"})
resp = urllib.request.urlopen(req2).read()
print("Job submitted:", resp.decode())

time.sleep(3)
allocs = json.loads(urllib.request.urlopen("http://localhost:4646/v1/job/test-artifact/allocations").read())
print("Allocs:", allocs[0]["ClientStatus"])
if "TaskStates" in allocs[0]:
    for tname, state in allocs[0]["TaskStates"].items():
        print(tname, state.get("Events", [])[-1])
