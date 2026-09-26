import datetime
import json
import os
import subprocess
import sys
import time
import urllib.parse
import urllib.request

url = os.environ["REFORGE_URL"].rstrip("/")
org = os.environ["REFORGE_ORG"]
repo_id = os.environ["REFORGE_QUALIFY_REPOSITORY_ID"]
cookie = os.environ["REFORGE_SESSION_COOKIE"]
since = os.environ.get("REFORGE_QUALIFY_SINCE") or (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=1)).isoformat(timespec="seconds")
wait = int(os.environ.get("REFORGE_QUALIFY_WAIT_SECONDS", "0"))


def api(path, query=None):
    target = f"{url}/api/v1/orgs/{org}{path}"
    if query:
        target += "?" + urllib.parse.urlencode(query)
    request = urllib.request.Request(target, headers={"Cookie": cookie, "Accept": "application/json"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def gh(*args):
    return json.loads(subprocess.run(["gh", *args], check=True, capture_output=True, text=True).stdout or "null")


def merges():
    events, cursor = [], None
    while True:
        query = {"repository_id": repo_id, "action": "merge.merged", "since": since, "limit": 100}
        if cursor:
            query["cursor"] = cursor
        page = api("/audit-events", query)
        events += page["items"]
        cursor = page.get("next_cursor")
        if page.get("complete", True) or not cursor:
            return events


def release(name, sha):
    runs = gh("run", "list", "-R", name, "--commit", sha, "--event", "push", "--json", "status,conclusion,workflowName,url")
    if not runs:
        return "pending", "no push run yet"
    if any(r["status"] != "completed" for r in runs):
        return "pending", "push run in progress"
    failed = [r for r in runs if r["conclusion"] != "success"]
    if failed:
        return "fail", f"{failed[0]['workflowName']} {failed[0]['conclusion']} {failed[0]['url']}"
    return "pass", ", ".join(sorted({r["workflowName"] for r in runs}))


name = api(f"/repositories/{repo_id}")["name"]
deadline = time.time() + wait
while True:
    results = []
    for event in merges():
        operation = api(f"/merge-operations/{event['object_id']}")
        pr = gh("pr", "view", operation["change_id"], "-R", name, "--json", "state,mergeCommit,title")
        sha = (pr.get("mergeCommit") or {}).get("oid") or (operation.get("native_result") or {}).get("merge_sha", "")
        if pr["state"] != "MERGED" or not sha:
            results.append(("fail", operation["change_id"], pr["title"], "not merged on the forge"))
            continue
        state, detail = release(name, sha)
        results.append((state, operation["change_id"], pr["title"], detail))
    if any(r[0] == "pass" for r in results) or time.time() >= deadline:
        break
    time.sleep(60)

for state, change, title, detail in results:
    print(f"{state:7} #{change} {title} — {detail}")
if not results:
    print(f"No Reforge merges in {name} since {since}")
sys.exit(0 if any(r[0] == "pass" for r in results) and not any(r[0] == "fail" for r in results) else 1)
