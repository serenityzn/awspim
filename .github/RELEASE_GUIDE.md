# Release Guide

This document explains how to create releases for the AWS PIM project using the automated GoReleaser pipeline.

## Overview

The project uses GoReleaser with GitHub Actions to automatically build and publish releases when a new version tag is pushed.

## Prerequisites

- Write access to the repository
- Git installed locally
- Proper commit permissions

## Creating a Release

### 1. Prepare Your Code

Ensure all changes are committed and pushed to the `main` branch:

```bash
git checkout main
git pull origin main
```

### 2. Create and Push a Version Tag

The release pipeline triggers on tags matching the pattern `vX.Y.Z` (e.g., `v1.0.0`, `v2.1.3`).

```bash
# Create a new tag (replace with your version)
git tag -a v1.0.0 -m "Release version 1.0.0"

# Push the tag to GitHub
git push origin v1.0.0
```

### 3. Monitor the Release

1. Go to your repository on GitHub
2. Navigate to **Actions** tab
3. You should see the **Release** workflow running
4. Wait for it to complete (usually 2-5 minutes)

### 4. Verify the Release

Once the workflow completes:

1. Go to the **Releases** page on GitHub
2. You should see your new release with:
   - Release notes (auto-generated from commits)
   - Binary artifacts for multiple platforms:
     - `awspim_Linux_x86_64.tar.gz`
     - `awspim_Linux_arm64.tar.gz`
     - `awspim_Darwin_x86_64.tar.gz` (macOS Intel)
     - `awspim_Darwin_arm64.tar.gz` (macOS Apple Silicon)
     - `awspim_Windows_x86_64.zip`
     - `awspim_Windows_arm64.zip`
   - `checksums.txt` for verification

## Release Artifacts

Each release includes:

### Binaries
- **Linux** (amd64, arm64) - Static binaries with no dependencies
- **macOS** (Intel, Apple Silicon) - Universal compatibility
- **Windows** (amd64, arm64) - Ready-to-run executables

### Additional Files
- `README.md` - Project documentation
- `LICENSE` - License information
- `checksums.txt` - SHA256 checksums for verification

## Versioning Strategy

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR** version (`v2.0.0`) - Incompatible API changes
- **MINOR** version (`v1.1.0`) - New functionality (backward compatible)
- **PATCH** version (`v1.0.1`) - Bug fixes (backward compatible)

## Commit Message Convention

For better release notes, follow conventional commits:

- `feat: Add new feature` → Goes to "Features" section
- `fix: Fix bug` → Goes to "Bug fixes" section
- `enhance: Improve performance` → Goes to "Enhancements" section
- Other commits → Goes to "Others" section

Excluded from changelog:
- `docs:` - Documentation changes
- `test:` - Test changes
- `ci:` - CI/CD changes
- `chore:` - Maintenance tasks
- `style:` - Code style changes

## Example Workflow

```bash
# 1. Make your changes
git add .
git commit -m "feat: add support for multi-region AWS sessions"
git push origin main

# 2. Create a release tag
git tag -a v1.2.0 -m "Release v1.2.0: Multi-region support"
git push origin v1.2.0

# 3. Wait for GitHub Actions to complete
# 4. Check the Releases page for your new release
```

## Troubleshooting

### Release Failed

1. Check the GitHub Actions logs
2. Common issues:
   - **Build errors**: Fix code and create a new tag
   - **Permission errors**: Ensure `GITHUB_TOKEN` has write permissions (should be automatic)
   - **Invalid tag format**: Must match `vX.Y.Z` pattern

### Deleting a Failed Release

```bash
# Delete remote tag
git push --delete origin v1.0.0

# Delete local tag
git tag -d v1.0.0

# Fix issues and recreate the tag
```

### Pre-releases

Tags with suffixes are automatically marked as pre-releases:
- `v1.0.0-alpha.1`
- `v1.0.0-beta.1`
- `v1.0.0-rc.1`

## Testing Locally

You can test the GoReleaser configuration locally (requires GoReleaser installed):

```bash
# Install GoReleaser (macOS)
brew install goreleaser

# Validate configuration
goreleaser check

# Build snapshot (doesn't publish)
goreleaser release --snapshot --clean --skip=publish
```

## CI/CD Workflows

### Release Workflow (`.github/workflows/release.yml`)
- **Trigger**: Push of tags matching `v*.*.*`
- **Actions**: Builds binaries, creates GitHub release with artifacts

### Test Workflow (`.github/workflows/test.yml`)
- **Trigger**: Push to `main` or pull requests
- **Actions**: Runs tests, builds, creates snapshot (without publishing)

## Configuration Files

- `.goreleaser.yml` - GoReleaser configuration
- `.github/workflows/release.yml` - Release automation
- `.github/workflows/test.yml` - Build and test automation

## Support

For issues with the release process, check:
1. [GoReleaser Documentation](https://goreleaser.com/)
2. [GitHub Actions Documentation](https://docs.github.com/en/actions)
3. Project repository issues

