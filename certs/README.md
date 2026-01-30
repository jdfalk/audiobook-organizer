# TLS Certificates

This directory contains self-signed "snake oil" certificates for **development and testing only**.

## ⚠️ WARNING: DO NOT USE IN PRODUCTION

The included certificates (`localhost.crt` and `localhost.key`) are:
- **Self-signed** (browsers will show security warnings)
- **Publicly distributed** in this repository (not secret)
- **Only valid for localhost**
- **Intended for development/testing only**

## For Development

To run the server with HTTP/2 support using the included certificates:

```bash
./audiobook-organizer serve --tls-cert=certs/localhost.crt --tls-key=certs/localhost.key
```

Your browser will show a security warning because the certificate is self-signed. This is expected and safe for local development.

## For Production

**You MUST replace these certificates with proper TLS certificates** from a trusted Certificate Authority (CA).

### Option 1: Let's Encrypt (Free, Automated)

Use [Certbot](https://certbot.eff.org/) to get free certificates:

```bash
# Install certbot (varies by OS)
sudo apt install certbot  # Ubuntu/Debian
brew install certbot      # macOS

# Get certificate (replace yourdomain.com)
sudo certbot certonly --standalone -d yourdomain.com

# Certificates will be in /etc/letsencrypt/live/yourdomain.com/
# Use:
#   --tls-cert=/etc/letsencrypt/live/yourdomain.com/fullchain.pem
#   --tls-key=/etc/letsencrypt/live/yourdomain.com/privkey.pem
```

### Option 2: Commercial Certificate

1. Purchase certificate from a CA (DigiCert, Sectigo, etc.)
2. Follow their instructions to generate CSR and obtain certificate
3. Place certificate and key in a secure location (NOT in this directory)
4. Update file permissions: `chmod 600 /path/to/private.key`

### Option 3: Corporate/Internal CA

If your organization has an internal CA:
1. Request a certificate from your IT/Security team
2. Install the certificate and private key
3. Ensure the CA certificate is trusted by clients

## Regenerating Development Certificates

If you need to regenerate the snake oil certificates:

```bash
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout certs/localhost.key \
  -out certs/localhost.crt \
  -days 365 \
  -subj "/CN=localhost"
```

## Security Best Practices

- **Never commit production certificates to version control**
- Keep private keys secure (chmod 600)
- Use strong passwords for private keys in production
- Rotate certificates before expiration
- Monitor certificate expiration dates
- Use automated renewal (e.g., certbot renew)
