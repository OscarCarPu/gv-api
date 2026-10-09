// Package printers controls the house 3D printers (Domotics section): PrusaLink telemetry, job
// control and file management, the camera, and server-side recordings.
//
// Endpoints (semiprivate auth):
//
//	GET    /domotics/printers
//	GET    /domotics/printers/{id}/status
//	DELETE /domotics/printers/{id}/job
//	GET    /domotics/printers/{id}/files
//	PUT    /domotics/printers/{id}/files                  - upload, name in X-File-Name
//	PATCH  /domotics/printers/{id}/files                  - one chunk of a large upload
//	POST   /domotics/printers/{id}/files?name=            - start printing
//	DELETE /domotics/printers/{id}/files?name=
//	GET    /domotics/printers/{id}/files/progress[?u=]
//	GET    /domotics/printers/{id}/camera                 - one JPEG frame
//	GET    /domotics/printers/{id}/recordings
//	POST   /domotics/printers/{id}/recordings?action=start|stop
//	DELETE /domotics/printers/{id}/recordings?name=
//
// Public, signed by the URLs the recordings list returns:
//
//	GET    /domotics/printers/{id}/recordings/{name}
package printers
