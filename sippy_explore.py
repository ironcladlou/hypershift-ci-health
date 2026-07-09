#!/usr/bin/env -S uv run
"""Quick utility for exploring Sippy API responses."""
import json
import sys
import urllib.request
import urllib.parse

BASE = "https://sippy.dptools.openshift.org"

def fetch(path, **params):
    if params:
        path = f"{path}?{urllib.parse.urlencode(params)}"
    url = f"{BASE}{path}"
    print(f"GET {url}", file=sys.stderr)
    with urllib.request.urlopen(url) as r:
        return json.loads(r.read())

def pp(data):
    print(json.dumps(data, indent=2))

def summary(data):
    if isinstance(data, list):
        print(f"Array of {len(data)} items")
        if data:
            print(f"First item keys: {list(data[0].keys()) if isinstance(data[0], dict) else type(data[0])}")
    elif isinstance(data, dict):
        print(f"Object with keys: {list(data.keys())}")
        for k, v in data.items():
            if isinstance(v, list):
                print(f"  {k}: array[{len(v)}]")
            elif isinstance(v, dict):
                print(f"  {k}: object({list(v.keys())[:5]}...)")
            else:
                print(f"  {k}: {repr(v)[:80]}")

def hypershift_filter():
    return json.dumps({"items": [{"columnField": "name", "operatorValue": "contains", "value": "hypershift-main"}]})

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: sippy_explore.py <path> [key=value ...]")
        print("Examples:")
        print("  ./sippy_explore.py /api/jobs release=Presubmits limit=5")
        print("  ./sippy_explore.py /api/jobs/analysis release=Presubmits period=hour")
        print("  ./sippy_explore.py /api/tests/recent_failures release=Presubmits period=4h")
        sys.exit(1)

    path = sys.argv[1]
    params = {}
    for arg in sys.argv[2:]:
        k, v = arg.split("=", 1)
        params[k] = v

    data = fetch(path, **params)
    summary(data)
    print("---")
    pp(data)
