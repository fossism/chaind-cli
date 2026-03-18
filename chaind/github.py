import json
import requests
from datetime import datetime
from .db import CONFIG_DIR
from .models import Task

CONFIG_FILE = CONFIG_DIR / "config.json"

def save_config(token: str, username: str):
    with open(CONFIG_FILE, "w") as f:
        json.dump({"token": token, "username": username}, f)

def load_config():
    if not CONFIG_FILE.exists():
        return None
    with open(CONFIG_FILE, "r") as f:
        return json.load(f)

def fetch_assigned_tasks():
    config = load_config()
    if not config:
        raise Exception("Not authenticated. Run `chaind auth` first.")
    
    headers = {
        "Authorization": f"token {config['token']}",
        "Accept": "application/vnd.github.v3+json"
    }

    # Search for issues and PRs assigned to user or authored by user and open
    query = f"is:open assignee:{config['username']}"
    url = f"https://api.github.com/search/issues?q={query}&per_page=50"

    response = requests.get(url, headers=headers)
    response.raise_for_status()
    data = response.json()
    
    tasks = []
    for item in data.get("items", []):
        is_pr = "pull_request" in item
        
        repo_url = item.get("repository_url", "")
        repo = repo_url.replace("https://api.github.com/repos/", "")
        
        # parse the body, avoid long bodies
        body_text = item.get("body", "") or ""
        body_summary = body_text[:200]
        
        task = Task(
            github_id=item["id"],
            repo=repo,
            number=item["number"],
            title=item["title"],
            body=body_summary,
            state=item["state"],
            type="pr" if is_pr else "issue",
            html_url=item["html_url"],
            created_at=datetime.strptime(item["created_at"], "%Y-%m-%dT%H:%M:%SZ"),
            updated_at=datetime.strptime(item["updated_at"], "%Y-%m-%dT%H:%M:%SZ"),
            local_status="todo"
        )
        tasks.append(task)
    return tasks
