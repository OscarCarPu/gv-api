# gv-api

Un orquestador de vida integral desarrollado en Go, diseñado para centralizar datos de múltiples servicios web, plataformas y dispositivos.

## Coverage

| File | Coverage | Comments |
| :--- | :---: | :--- |
| `gv-api/cmd/api/main.go` | ![skipped](https://img.shields.io/badge/SKIPPED-grey) | Entry point, no business logic |
| `gv-api/internal/config/config.go` | ![skipped](https://img.shields.io/badge/SKIPPED-grey) | Configuration boilerplate |
| `gv-api/internal/database/db.go` | ![skipped](https://img.shields.io/badge/SKIPPED-grey) | Database boilerplate |
| `gv-api/internal/habits/handler.go` | ![100.0%](https://img.shields.io/badge/100.0%25-brightgreen) |  |
| `gv-api/internal/habits/repository.go` | ![100.0%](https://img.shields.io/badge/100.0%25-brightgreen) |  |
| `gv-api/internal/habits/service.go` | ![100.0%](https://img.shields.io/badge/100.0%25-brightgreen) |  |
| `gv-api/internal/response/response.go` | ![87.5%](https://img.shields.io/badge/87.5%25-brightgreen) |  |
| `gv-api/test/e2e/client.go` | ![skipped](https://img.shields.io/badge/SKIPPED-grey) | E2E test infrastructure |
| `gv-api/test/e2e/setup.go` | ![skipped](https://img.shields.io/badge/SKIPPED-grey) | E2E test infrastructure |
| **Total** | ![98.2%](https://img.shields.io/badge/98.2%25-brightgreen) | Excludes boilerplate and test infra |

## Tecnologías

- **Go:** Core del sistema.
- **`go-chi/chi/v5`:** Router ligero e idiomático.
- **`pgx/v5` & `sqlc`:** Interacción con PostgreSQL eficiente y generación automática de código.
- **PostgreSQL:** Almacenamiento relacional de métricas y configuraciones.

## Instalación y Configuración

### Requisitos
- Git, Docker & Docker Compose
- Go (v1.25.6+)
- sqlc

### Pasos iniciales

1. **Clonar y configurar:**
   ```bash
   git clone [https://github.com/OscarCarPu/gv-api.git](https://github.com/OscarCarPu/gv-api.git)
   cd gv-api
   make setup-project
