# Printers (Domotics)

PrusaLink telemetry, job control, print files, camera and recordings for the house printers. **Semiprivate auth** (full or semiprivate token), except the signed media URLs below.

```
client → gv-api → PrusaLink (HTTP Digest) / ffmpeg (RTSP) / disk
```

Config is env-only (`PRINTER_RTSP_URL`, `PRUSALINK_*`, `PRINTER_RECORDING*`; see `.env.example`). Credentials never leave the API. One printer today, `core-one`.

## Endpoints

| Endpoint | Notes |
|---|---|
| `GET /domotics/printers` | `[{id, name, model}]` |
| `GET /domotics/printers/{id}/status` | Telemetry. Always 200; `online:false` + `error` when PrusaLink is down |
| `DELETE /domotics/printers/{id}/job` | Stops the running print. The API asks the printer for the job id itself; 409 when idle |
| `GET /domotics/printers/{id}/history` | Last 10 prints from Prusa Connect: `[{state, timePrinting, fileName, material, filamentGrams, filamentMeters}]`. 503 without `PRUSA_CONNECT_USER/PASSWORD`; 502 when the Prusa login or Connect fails |
| `GET /domotics/printers/{id}/files` | Files on the printer's storage, with free space. Each file carries a signed `downloadUrl` |
| `GET …/files/meta` | `{files: {<8.3 name>: {printSeconds, material, filamentGrams, filamentMeters}}}` from Prusa Connect, separate so the list never waits on Connect (see notes). 503 without Connect |
| `GET …/chamber` | `{temp, targetTemp}` from Prusa Connect (PrusaLink does not expose it), cached 10s. 503 without Connect |
| `PUT …/files` | Upload. Name in `X-File-Name` (URL-encoded), optional `X-Upload-Id`, `?overwrite=1`. 202 once the bytes are staged |
| `PATCH …/files` | One chunk of a large upload: `X-Upload-Id`, `X-Upload-Offset`, `X-Upload-Total`. Chunks are in order and idempotent; 409 carries `received`. 202 on the last one |
| `POST …/files?name=` | Start printing a file |
| `DELETE …/files?name=` | Delete a file |
| `POST …/files/rename` | `{name, newName}`. PrusaLink cannot rename, so the API downloads the file, uploads it under the new name and deletes the original, as a queued transfer: 202 `{uploadId, name}`, then follow it on `/files/progress`. The extension is kept (added when missing); 409 when the name is taken, the file is read-only or printing |
| `GET …/files/download?name&as&exp&sig` | **No bearer.** Streams the file from the printer as an attachment named `as` |
| `GET …/files/progress[?u=id]` | Progress of one transfer, or every transfer still known: `status` is `queued` → (`downloading` for a rename) → `forwarding` → `done`/`error`; a rename also has `from` |
| `GET …/camera` | One JPEG frame from a warm ffmpeg process that idles out after 5 min without viewers. A cold start takes ~2–3s (RTSP + first keyframe), so the first frame may take up to 20s before giving up; `/camera/url` already starts it |
| `GET …/camera/url` | `{url}`: signed link to the stream below, valid 12–13h |
| `GET …/camera/stream?exp&sig` | **No bearer.** `multipart/x-mixed-replace` MJPEG, ~5 fps, runs until the client leaves |
| `GET …/recordings` | Recordings on disk plus the one being written, with signed `url` / `posterUrl` |
| `POST …/recordings?action=start\|stop` | Recording is a server job and outlives the client |
| `DELETE …/recordings?name=[&name=…]` | Delete one (409 while it is recording) or several: `{deleted, failed: {name: error}}` |
| `GET …/recordings/{name}?exp&sig` | **No bearer.** Signed with the JWT secret, valid 12–13h. Honours `Range`; `&download=1` adds `Content-Disposition` |

## On the backup server

With `FAILOVER_SIDE=aws` every endpoint here answers `503` before reaching the handler (after auth, except for signed media URLs):

```json
{"error": "Not available while running on the backup server. Back when home is restored.", "code": "unavailable_on_failover"}
```

## Notes

- Print metadata: Buddy firmware (CORE One) does not return the `meta` object the PrusaLink spec documents, and its download ignores `Range`, so `/files/meta` reads Prusa Connect's `/app/printers/{uuid}/files` (`PRINT_FILE` entries, keyed by the basename of the 8.3 `path`, same as `name` in the PrusaLink list). A just-uploaded file gets it once the printer has synced it to Connect (a minute or two). History amounts for a stopped/failed job are pro-rated by `progress`, or by printing time over the estimate.
- A rename copies the whole file through the printer's USB read (~70 KB/s while printing), so a big file takes minutes and holds the transfer queue meanwhile.
- Transfers to and from the printer have no fixed deadline (USB is ~70 KB/s while printing): they are cut only when no bytes move for 90s, or 10 min after the whole upload body is sent and the printer is still answering, with a cap of max(1h, size ÷ 4 KB/s).
- Transfers to a printer run one at a time, in arrival order (a per-printer queue of 32); a full queue answers 503.
- Large uploads must be chunked: the tunnel in front of the API rejects bodies over 100 MB. Chunks stage on disk (`PRINTER_UPLOADS_DIR`) and the assembled file streams to the printer in the background, so the request returns before the slow USB write.
- Recording is automatic: a watcher polls the printer every 5s and starts a recording when its state turns `PRINTING` (a pause keeps it going) and stops it when the print ends. It only stops a recording it started, treats the first reading after boot as a baseline (a restart mid-print starts nothing) and ignores offline polls. It does not run on the failover instance.
- Recordings are their own metadata: name = UTC start time, mtime = end, size = size. Fragmented MP4, so a killed ffmpeg leaves a playable file. The directory must be a volume (`printer_recordings` in compose, owned by uid 1000).
- The status overlay burned into recordings (`PRINTER_RECORDING_OVERLAY`) needs a libx264 encode; `0` goes back to `-c:v copy`.
- Print history logs in to Prusa Connect with the account email and password (OAuth code + PKCE against account.prusa3d.com, the same flow as the website), keeps the access token in memory, refreshes it with the refresh token and logs in again if that fails. From an IP Prusa does not know it asks for a code sent by email: run `make prusa-login` in gv-api, which logs in from the running container, prompts for the code and keeps the refresh token in `PRUSA_CONNECT_TOKEN_FILE` (the `prusa_connect` volume). Until then the history answers 502 asking for it. It is unofficial and can break when Prusa changes the site.
- An upstream 401 from PrusaLink is returned as 502, so a bad printer password never reads as an expired session.
- `scripts/fake_prusalink.py` stands in for the printer: `PRUSALINK_HOST=http://127.0.0.1:8899 PRUSALINK_USER=maker PRUSALINK_PASSWORD=test1234`.
