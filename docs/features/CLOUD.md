# AWS failover plan 

## Idea

- **Lambda** is the only brain. It runs every 1-5 minutes, keeps a state in SSM, and does every step: start EC2, check health, flip the CNAME, stop the API, dump, shut down, and send Telegram messages.
- **EC2** is a passive worker. It restores on boot, runs the app, and runs commands Lambda sends through SSM Run Command.
- **Cloudflare** provides two tunnels, the DNS API, and fixed health hostnames.
- **Home Lab** restores the dump into Psg when it restarts, and lets the user chose what changes that weren't dump stay or leave 

## Setup

### Backups

- Put with already existing backup logic, upload also to s3, decide if it goes to s3 daily or hourly (review costs)

### AWS

- Stopped EC2 (x86, Docker) with the same compose stack and `cloudflared` for tunnel alt. No inbound ports, access through SSM Session Manager.
- EC2 boot unit: restore the newest S3 dump -> start compose -> start cloudflared.
- Lambda (not in a VPC), triggered by EventBridge every 1-5 min. Permissions: start/stop that one instance, SSM SendCommand, read/write the state parameter, read the Cloudflare and Telegram tokens from SSM SecureString.
- SSM parameters: `state`, fail and ok counters, and a timestamp for each state entry.

### Cloudflare

- Two tunnels (home, alt). The app hostname is a proxied CNAME that points to one of them.
- API token scoped to DNS edit, stored only in SSM and read by Lambda.

## Lambda logic (state machine, one step per run)

| State | What Lambda does each run |
|---|---|
| `NORMAL` | Check `home-health`. After 10 minutes of down -> start EC2, set `STARTING`. |
| `STARTING` | Wait until `alt-health` is OK (the boot unit restored the DB). Then flip the CNAME to alt, send Telegram , set `FAILED_OVER`. |
| `FAILED_OVER` | Check `home-alive`. After 10 consecutive minutes of OK -> set `FAILING_BACK` and send the first failback command. |
| `FAILING_BACK` | 1. Stop the API on EC2 through SSM Run Command. 2. Dump the EC2 DB to S3 as the failback dump. 3. Mark the dump ready. 4. Wait for the home agent to report db restored  
| `HOME_READY` | Flip the CNAME to home, check `home-health`, stop cloudflared and shut down EC2, send Telegram , set `NORMAL`. |

## Planned maintenance

- Button on gv-web: take a final backup, set state = PLANNED, then shuts down the home lab server

## Cost estimated 

### Normal:
- EBs disk, 8 GB gp3 .7$/month (system prune from time to time)
- S3, ~2GB 0.04$/month
- S3 requests (hourly uploads) 0.004$/month 
- Lambda, EventBrdige, SSM 0$ 
Total 0.75$

### One 24-hour failover 
- t3.small 0.55$
- public ipv4 while running 0.12$
- detection, restore, and failback lambda cost 0.1$
data transfer 0$ 
extra cost of outage 1$
