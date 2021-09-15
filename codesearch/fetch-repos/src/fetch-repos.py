import requests
import json
import os

REPOS = [
    'kubernetes',
    'kubernetes-client',
    'kubernetes-csi',
    'kubernetes-incubator',
    'kubernetes-sigs',
]

CONFIG = {
    "max-concurrent-indexers": 10,
    "dbpath": "data",
    "vcs-config": {
        "git": {
            "detect-ref" : True,            
            "ref" : "main"
        }
    },
    "repos": {}
}

def fetch_repos():
    print("Starting Fetch Repo Script..")

    file_path = "/data/config.json"
    if os.environ.get('CONFIG_PATH') is not None:
        file_path = os.environ.get('CONFIG_PATH')

    for repo in REPOS:    
        resp = requests.get(url= "https://api.github.com/orgs/" + repo + "/repos?per_page=200")
        data = resp.json()

        for item in data:
            name = item['full_name'].split('/')[1]
            CONFIG["repos"][repo + "/" + name] = {
                "url": "https://github.com/%s/%s.git" % (repo, name),
                "ms-between-poll": 360000
            }

    with open(file_path, 'w') as f:
        f.write(json.dumps(CONFIG, indent=4, sort_keys=True))
    print("File config saved to: %s" % file_path)

if __name__ == "__main__":
    fetch_repos()