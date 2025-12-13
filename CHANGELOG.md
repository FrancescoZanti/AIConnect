# Changelog

Tutte le modifiche rilevanti a questo progetto vengono documentate in questo file.

Il formato è basato su [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e questo progetto segue il versioning SemVer.

## [Unreleased]

### Added

- Wizard CLI per generare la configurazione quando mancante o non compilata (placeholder), utilizzabile anche in container.

## [0.0.1] - 2025-12-13

### Added (0.0.1)

- Release automatica via GitHub Actions basata sul file `VERSION`.
- Build e pubblicazione di pacchetti RPM (Fedora e RHEL 10) come asset della GitHub Release.
- Build e pubblicazione dell’immagine container su GHCR con tag `v<versione>` e `latest`.
