# GPG Signing Guide for Releases

This guide explains how to set up GPG signing for your releases to ensure package authenticity and integrity.

## 📋 What is GPG Signing?

GPG (GNU Privacy Guard) signing allows users to verify that:
- ✅ Packages were released by you (authenticity)
- ✅ Packages haven't been tampered with (integrity)
- ✅ Packages came from the official source (non-repudiation)

## 🔑 Step 1: Generate GPG Key

### Install GPG

**macOS:**
```bash
brew install gnupg
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt-get install gnupg
```

**Linux (CentOS/RHEL):**
```bash
sudo yum install gnupg
```

### Generate Your Key

```bash
# Generate a new GPG key
gpg --full-generate-key
```

Follow the prompts:
1. **Key type**: Select `(1) RSA and RSA` (default)
2. **Key size**: Enter `4096` (recommended for strong security)
3. **Expiration**: Enter `0` (never expires) or set a specific time (e.g., `2y` for 2 years)
4. **Real name**: Your name or organization name (e.g., "AWS PIM Project")
5. **Email**: Your email address (e.g., "awspim@company.com")
6. **Comment**: Optional (e.g., "Release signing key")
7. **Passphrase**: Enter a strong passphrase (you'll need this for GitHub Secrets)

Example output:
```
pub   rsa4096 2024-01-01 [SC]
      1234567890ABCDEF1234567890ABCDEF12345678
uid           AWS PIM Project <awspim@company.com>
sub   rsa4096 2024-01-01 [E]
```

The long hex string is your **GPG Key ID/Fingerprint**.

## 🔐 Step 2: Export GPG Key

### Export Private Key

```bash
# Replace with your key fingerprint or email
gpg --armor --export-secret-keys awspim@company.com > private-key.asc
```

⚠️ **IMPORTANT**: Keep this file secure! Never commit it to git!

### Export Public Key

```bash
# Export public key
gpg --armor --export awspim@company.com > public-key.asc
```

### Get Key Fingerprint

```bash
# List keys and get fingerprint
gpg --list-secret-keys --keyid-format=long
```

Output example:
```
sec   rsa4096/ABCDEF1234567890 2024-01-01 [SC]
      1234567890ABCDEF1234567890ABCDEF12345678
uid                 [ultimate] AWS PIM Project <awspim@company.com>
```

The fingerprint is: `1234567890ABCDEF1234567890ABCDEF12345678`

## 🚀 Step 3: Add Secrets to GitHub

1. **Go to your GitHub repository**
2. **Navigate to**: Settings → Secrets and variables → Actions
3. **Click**: "New repository secret"

### Add GPG_PRIVATE_KEY

- **Name**: `GPG_PRIVATE_KEY`
- **Value**: Paste the entire contents of `private-key.asc` (including the BEGIN/END lines)

```
-----BEGIN PGP PRIVATE KEY BLOCK-----

[your private key content]

-----END PGP PRIVATE KEY BLOCK-----
```

### Add GPG_PASSPHRASE

- **Name**: `GPG_PASSPHRASE`
- **Value**: The passphrase you set when generating the key

## 📤 Step 4: Publish Public Key (Optional but Recommended)

Users need your public key to verify signatures. Publish it to make it easy to find:

### Option 1: Public Keyservers

```bash
# Upload to keyserver
gpg --keyserver keys.openpgp.org --send-keys 1234567890ABCDEF1234567890ABCDEF12345678

# Or use Ubuntu keyserver
gpg --keyserver keyserver.ubuntu.com --send-keys 1234567890ABCDEF1234567890ABCDEF12345678
```

### Option 2: GitHub

```bash
# Copy your public key
gpg --armor --export awspim@company.com
```

Then add it to your repository:
1. Create a file `GPG_PUBLIC_KEY.asc` in your repo root
2. Paste the public key contents
3. Commit and push

### Option 3: README

Add your key fingerprint and keyserver info to your README:

```markdown
## 🔐 GPG Verification

All releases are signed with GPG key:
- **Fingerprint**: `1234 5678 90AB CDEF 1234  5678 90AB CDEF 1234 5678`
- **Key ID**: `ABCDEF1234567890`

Import the public key:
```bash
gpg --keyserver keys.openpgp.org --recv-keys ABCDEF1234567890
```
```

## ✅ Step 5: Test Locally

Before pushing, test signing locally:

```bash
# Build a snapshot release (doesn't publish)
goreleaser release --snapshot --clean --skip=publish

# Check that signature file was created
ls dist/checksums.txt.sig

# Verify the signature
gpg --verify dist/checksums.txt.sig dist/checksums.txt
```

Expected output:
```
gpg: Signature made Mon 01 Jan 2024 12:00:00 PM EST
gpg:                using RSA key 1234567890ABCDEF1234567890ABCDEF12345678
gpg: Good signature from "AWS PIM Project <awspim@company.com>" [ultimate]
```

## 🎯 Step 6: Create a Signed Release

```bash
# Commit the workflow changes
git add .github/workflows/release.yml .goreleaser.yml
git commit -m "feat: add GPG signing to releases"
git push origin main

# Create and push a tag
git tag -a v0.2.0 -m "Release v0.2.0 with GPG signing"
git push origin v0.2.0
```

The release will now include:
- ✅ `checksums.txt` - SHA256 checksums
- ✅ `checksums.txt.sig` - GPG signature for the checksums

## 🔍 How Users Verify Signatures

### Step 1: Download Files

```bash
# Download release files
wget https://github.com/serenityzn/awspim/releases/download/v0.2.0/awspim_Linux_x86_64.tar.gz
wget https://github.com/serenityzn/awspim/releases/download/v0.2.0/checksums.txt
wget https://github.com/serenityzn/awspim/releases/download/v0.2.0/checksums.txt.sig
```

### Step 2: Import Your Public Key

```bash
# Import from keyserver (replace with your key ID)
gpg --keyserver keys.openpgp.org --recv-keys ABCDEF1234567890

# Or import from file if you provide it
gpg --import GPG_PUBLIC_KEY.asc
```

### Step 3: Verify Signature

```bash
# Verify the GPG signature
gpg --verify checksums.txt.sig checksums.txt
```

Expected output:
```
gpg: Signature made Mon 01 Jan 2024 12:00:00 PM EST
gpg:                using RSA key 1234567890ABCDEF1234567890ABCDEF12345678
gpg: Good signature from "AWS PIM Project <awspim@company.com>"
```

### Step 4: Verify Checksums

```bash
# Verify file integrity
sha256sum -c checksums.txt 2>&1 | grep awspim_Linux_x86_64.tar.gz
```

Expected output:
```
awspim_Linux_x86_64.tar.gz: OK
```

## 🔄 Key Rotation Best Practices

### When to Rotate Keys

- Every 1-2 years (if you set expiration)
- If your key is compromised
- When changing maintainers

### How to Rotate

1. **Generate new key** (follow Step 1)
2. **Update GitHub secrets** with new key
3. **Publish new public key**
4. **Revoke old key** (if compromised):
   ```bash
   gpg --gen-revoke OLD_KEY_ID > revocation.asc
   gpg --import revocation.asc
   gpg --keyserver keys.openpgp.org --send-keys OLD_KEY_ID
   ```
5. **Announce the change** in release notes

## 🛠️ Troubleshooting

### "No secret key" error

**Problem**: GitHub Actions can't find the GPG key
**Solution**: Verify `GPG_PRIVATE_KEY` secret is correctly set in GitHub

### "Signing failed" error

**Problem**: Wrong passphrase or fingerprint
**Solution**: 
- Check `GPG_PASSPHRASE` secret
- Verify the key was imported successfully
- Check workflow logs for details

### Users can't verify signatures

**Problem**: Public key not available
**Solution**: 
- Upload to multiple keyservers
- Provide public key in repository
- Document fingerprint in README

### "Bad signature" error

**Problem**: File was modified after signing
**Solution**: 
- Re-download the files
- Check if using correct version
- Report if files are compromised

## 📚 Additional Resources

- [GPG Documentation](https://gnupg.org/documentation/)
- [GitHub GPG Guide](https://docs.github.com/en/authentication/managing-commit-signature-verification)
- [GoReleaser Signing](https://goreleaser.com/customization/sign/)
- [OpenPGP Best Practices](https://riseup.net/en/security/message-security/openpgp/best-practices)

## 🔒 Security Notes

1. **Protect your private key**: Never share or commit it
2. **Use strong passphrase**: At least 20 characters, random
3. **Backup your key**: Store securely offline
4. **Document fingerprint**: Make it easy for users to find
5. **Monitor usage**: Watch for unauthorized signing attempts

## 💡 Quick Reference

```bash
# Generate key
gpg --full-generate-key

# Export private key
gpg --armor --export-secret-keys EMAIL > private-key.asc

# Export public key
gpg --armor --export EMAIL > public-key.asc

# Get fingerprint
gpg --list-secret-keys --keyid-format=long

# Upload to keyserver
gpg --keyserver keys.openpgp.org --send-keys KEY_ID

# Verify signature
gpg --verify checksums.txt.sig checksums.txt
```

