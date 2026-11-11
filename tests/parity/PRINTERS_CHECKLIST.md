# Printer checklist (real hardware)

Printer writes are not replayed by the parity runner: each one would reach the physical printer
once per app. They are checked here by hand, on the Python service, before the cutover.

## Setup

1. Point the Python app at the printer. Either run the dev stack on the Python image, or use the
   parity stack with `tests/parity/printer.env` (git-ignored), holding the printer lines of
   `.env`:

   ```
   PRINTER_RTSP_URL=rtsp://192.168.1.211/live
   PRUSALINK_HOST=http://192.168.1.235
   PRUSALINK_USER=maker
   PRUSALINK_PASSWORD=...
   PRUSA_CONNECT_USER=...
   PRUSA_CONNECT_PASSWORD=...
   ```

2. Log in to Prusa Connect once with `gv-api prusa-login` in the container (task 17.6), then
   check that `GET /domotics/printers/core-one/history` answers without restarting the app.
3. With `printer.env` in place, `make parity CORPUS=printers` reads the real printer from both
   apps (task 17.7).

## Checks

Tick each item with the date it passed (`- [x] 2026-11-12 …`).

- [ ] Upload a small file (one PUT); it appears in the file list.
- [ ] Upload a file over the chunk size (PATCH chunks); progress reaches `done` and it is listed.
- [ ] Rename a file; progress goes downloading → forwarding → done, and the old name is gone.
- [ ] Start a print from the list, then stop it (`DELETE …/job`).
- [ ] Take a camera frame (`GET …/camera`).
- [ ] Open the MJPEG stream from `GET …/camera/url` in a browser.
- [ ] Auto-record starts within seconds of a print starting, and stops when it ends.
- [ ] Download a recording and seek in it (Range requests); the poster shows in the list.
- [ ] Bulk-delete two recordings; the response lists both under `deleted`.
