# Contributing to dbseal 🦭

Thank you for your interest in contributing to **dbseal**!

## 📜 Contributor License Agreement (CLA)

`dbseal` is distributed under the **GNU Affero General Public License v3 (AGPL v3)**, alongside a commercial edition (`dbseal Pro`).

To allow us to distribute the software under this dual-license model while protecting the open-source project and commercial integrity, all contributors must agree to the Contributor License Agreement (CLA).

When you submit a Pull Request on GitHub, **CLA-Assistant** will automatically prompt you to review and sign the agreement with a single click.

## 🛠 Local Development Setup

1. **Prerequisites**:
   - Go 1.24+ installed
   - MySQL 5.7+ or 8.0+ / PostgreSQL 12+ running locally or in Docker

2. **Clone & Test**:
   ```bash
   git clone https://github.com/sealkit/dbseal.git
   cd dbseal

   # Run all unit tests
   go test -v ./...

   # Build the binary
   go build -o bin/dbseal.exe ./cmd/dbseal
   ```

3. **Code Style & Guidelines**:
   - Run `go fmt ./...` before committing.
   - Ensure all new features include unit tests in the respective `_test.go` file.
   - Keep driver implementations decoupled via `internal/drivers/driver.go`.
