import json
import sys
import urllib.request
from pathlib import Path

import pyotp


def load_env():
    env = {}
    env_path = Path(__file__).resolve().parent.parent / ".env"
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        env[key.strip()] = value.strip()
    return env


def main():
    env = load_env()
    for key in ("PORT", "PASSWORD", "TOTP_SECRET"):
        if key not in env:
            print(f"Missing {key} in .env", file=sys.stderr)
            sys.exit(1)

    base_url = f"http://127.0.0.1:{env['PORT']}"

    # Step 1: Login to get temp token
    req = urllib.request.Request(
        f"{base_url}/login",
        data=json.dumps({"password": env["PASSWORD"]}).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req) as resp:
        tmp_token = json.loads(resp.read())["token"]

    # Step 2: Generate TOTP code and do 2FA
    code = pyotp.TOTP(env["TOTP_SECRET"]).now()
    req = urllib.request.Request(
        f"{base_url}/login/2fa",
        data=json.dumps({"token": tmp_token, "code": code}).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req) as resp:
        token = json.loads(resp.read())["token"]

    print(token)


if __name__ == "__main__":
    main()
