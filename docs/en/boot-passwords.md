# Boot menu passwords

Menu protection does not protect image files themselves. Direct downloads, alternative boot scripts and other boot services remain outside its scope. Use a trusted provisioning network: DHCP, TFTP and the initial HTTP bootstrap are not authenticated by these password checks.

## Requirements

Enable admin HTTPS and use an iPXE build with HTTPS, POST parameters and masked password forms. The loader must trust the HTTPS certificate and validate the server address. The bundled old loader is not sufficient. Use a compatible `ipxe-legacy` build and test its keyboard support before deployment; physical regression testing is still pending.

Use a stable CA for routine certificate renewal, or pin the deployment's self-signed certificate. For CA or pinned-certificate rotation, distribute loaders trusting both old and new certificates before changing the server certificate, then remove the old trust. Do not disable TLS verification or weaken server cipher settings. Password requests go directly to the admin HTTPS listener; TLS termination at a reverse proxy is not supported for this endpoint.

Custom loaders are unsigned. Keep `secureboot-official` unchanged until a signed, compatible loader and its certificate trust have been tested. If a loader cannot authenticate, a protected boot fails; it never falls back to an unprotected image.

## Set or remove a password

In **Boot Menu**, set the optional global password. In an image's properties, set its optional image password. Both are disabled by default. Leave a password field blank to keep its current value; select **Remove** to disable protection. Setting a password requires opening the admin interface over HTTPS.

- A global password is required before entering the network-boot menu. Local boot remains available without it.
- An image password is requested when that image is selected. Without a global password, the menu remains browsable.
- With both configured, unlocking the menu does not unlock the image.
- Defaults, next-boot images and deployment-profile selections use the same image check. Cancelling or failing authentication does not consume a protected next-boot selection.
- Cancelling returns to the menu. Authentication or TLS failure never starts a protected image. A source address gets up to five password attempts per minute.

Passwords are stored as bcrypt hashes and are never returned by the API or included in boot scripts. The maximum length is 72 bytes. Test punctuation and your keyboard layout on real clients before deployment.

Image-authentication requests use a five-minute menu token, scoped to the client MAC and source address. Changing the global password or restarting the server invalidates it. MAC addresses are not device identities. Use **Lock network-boot menu** to leave the menu or authenticate again after expiry; expiry does not hide an already displayed menu.

The checks apply to ISO images and the global network-boot menu. There are no separate passwords for tools, SAN targets, disk-imaging jobs or Kubernetes node images; the global password covers entry to those boot paths.

## API

`PUT /api/theme` and `PUT /api/images?filename=...` accept `boot_password`. Omit it to preserve the password, supply a non-empty string to set it, or `""` to remove it. Responses contain `boot_password_required`, never the password or hash. Changing that status field directly has no effect.

Boot clients submit URL-encoded POST bodies to `/boot-auth` on the admin HTTPS port. Passwords and menu-session tokens must not appear in URLs. This endpoint grants no admin session or API access.

## Replicas

Upgrade every replica before enabling protection on the primary. Replicas copy the protection flags, but password hashes are not exposed through the read API or synchronised. Protected menus and images therefore remain blocked on replicas; boot those clients from the primary. Replica password verification needs a separate design before using this feature across sites.
