# 0009. OCR with OCRmyPDF and text extraction with Apache Tika

- Status: Accepted
- Date: 2026-10-06
- Deciders: veHRz

## Context

Document Mode indexes attachments, including scanned documents.

## Decision

Text extraction uses Apache Tika; OCR uses OCRmyPDF (Tesseract). Both run as
optional containers in the compose profile `documents`.

## Consequences

Proven, light and multilingual. Without the containers, Document Mode works
without OCR and says so.

## Alternatives considered

Neural OCR models: heavier, for a gain not needed at first.
