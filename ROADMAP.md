# Roadmap

## Image Digest Pinning
High value, improves correctness.

## Convergence Time Warnings
Detect deploys that never stabilize.

## Config/Secret Version Drift Detection
Classify versioned config/secret mismatches beyond exact-name compares.

## SQLite State Store
Persist timestamps and limited history.

## CI Validation Mode
Reuse logic during CI to catch bad deploys.

## Compose YAML Normalization
Normalize compose content (e.g., whitespace/comment-insensitive) before fingerprinting.

## File URL Compose Sources
Allow `file://` compose URLs for local development and testing.

## Optional Enforcement Mode
High risk, future only.
