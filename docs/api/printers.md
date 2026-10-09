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
| `GET /domotics/printers/{id}/files` | Files on the printer's storage, with free space |
| `PUT …/files` | Upload. Name in `X-File-Name` (URL-encoded), optional `X-Upload-Id`, `?overwrite=1`. 202 once the bytes are staged |
| `PATCH …/files` | One chunk of a large upload: `X-Upload-Id`, `X-Upload-Offset`, `X-Upload-Total`. Chunks are in order and idempotent; 409 carries `received`. 202 on the last one |
| `POST …/files?name=` | Start printing a file |
| `DELETE …/files?name=` | Delete a file |
| `GET …/files/progress[?u=id]` | Forwarding progress to the printer for one upload, or every upload still known |
| `GET …/camera` | One JPEG frame from a warm ffmpeg process that idles out after 30s |
| `GET …/camera/url` | `{url}`: signed link to the stream below, valid 12–13h |
| `GET …/camera/stream?exp&sig` | **No bearer.** `multipart/x-mixed-replace` MJPEG, ~5 fps, runs until the client leaves |
| `GET …/recordings` | Recordings on disk plus the one being written, with signed `url` / `posterUrl` |
| `POST …/recordings?action=start\|stop` | Recording is a server job and outlives the client |
| `DELETE …/recordings?name=` | Delete one (409 while it is recording) |
| `GET …/recordings/{name}?exp&sig` | **No bearer.** Signed with the JWT secret, valid 12–13h. Honours `Range`; `&download=1` adds `Content-Disposition` |

## Notes

- Large uploads must be chunked: the tunnel in front of the API rejects bodies over 100 MB. Chunks stage on disk (`PRINTER_UPLOADS_DIR`) and the assembled file streams to the printer in the background, so the request returns before the slow USB write.
- Recordings are their own metadata: name = UTC start time, mtime = end, size = size. Fragmented MP4, so a killed ffmpeg leaves a playable file. The directory must be a volume (`printer_recordings` in compose, owned by uid 1000).
- The status overlay burned into recordings (`PRINTER_RECORDING_OVERLAY`) needs a libx264 encode; `0` goes back to `-c:v copy`.
- An upstream 401 from PrusaLink is returned as 502, so a bad printer password never reads as an expired session.
- `scripts/fake_prusalink.py` stands in for the printer: `PRUSALINK_HOST=http://127.0.0.1:8899 PRUSALINK_USER=maker PRUSALINK_PASSWORD=test1234`.
