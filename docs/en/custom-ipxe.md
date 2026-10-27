# Custom iPXE with HTTPS

The bundled bootloaders stay unchanged. The opt-in builder uses iPXE revision `39a440d082a65a5b8e11e734cd6caf0991b70771`, with HTTPS, POST parameters and masked password forms enabled for BIOS and UEFI.

UEFI uses `ipxe-legacy` for both network and USB boot. It retains firmware drivers instead of taking over USB controllers. Native USB NIC drivers are excluded. Test your NIC, keyboard and the next bootloader before deploying it; physical validation of the reported keyboard regression is still pending.

## Build and install

Choose the public certificate to trust before building. Prefer a stable CA certificate whose key stays off the boot server. A deployment's self-signed server certificate also works, but replacing it requires rebuilding the loaders.

```sh
CONTAINER_ENGINE=podman bash scripts/build-modern-bootloaders.sh /tmp/bootimus-modern /path/to/ca.pem
```

The script needs Bash, OpenSSL, `sha256sum` and Docker or Podman. It refuses an existing output directory. It embeds public certificates and their trust fingerprints, never private keys. Check `SHA256SUMS` before copying the output to `<data>/bootloaders/modern/`. Select `modern` for a test client first. Use its `bootimus.usb` for USB testing; the admin UI's bundled USB download remains the old loader.

The build preserves the HTTP range fix used for large images. Its mtools version and iPXE revision are pinned; no experimental TLS or password-encoding patches are needed.

## Certificate changes

Configure Bootimus's admin HTTPS certificate and key as described in [deployment](deployment.md). The certificate must cover the server address used by boot clients. Serve any intermediate certificates with the leaf certificate. Embedding the trusted root avoids a dependency on an external certificate download service. This follows [iPXE's certificate configuration](https://ipxe.org/crypto).

Renewing a leaf certificate under the same trusted CA does not require new loaders. For CA rotation, pass both old and new public CA certificates to the builder, distribute those loaders, then change the server certificate. Rebuild without the old CA after migration. Use the same overlap procedure when rotating pinned self-signed certificates. Never disable certificate verification or enable obsolete server ciphers to keep an old loader working.

## Secure Boot and scope

These custom loaders are unsigned. They cannot replace `secureboot-official` on clients that enforce Secure Boot. Keep that signed set unchanged; a custom build needs a separate signing and enrolment procedure, followed by testing of the complete boot chain.

HTTPS certificate trust and Secure Boot signing trust are separate. The embedded script still discovers Bootimus over DHCP and fetches its bootstrap over TFTP/HTTP, so this setup assumes a trusted provisioning network. It does not authenticate that bootstrap or protect image downloads. A loader that cannot complete password authentication must stop at the protected menu or image, not fall back to an unprotected boot.
