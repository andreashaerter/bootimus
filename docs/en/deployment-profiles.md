# Deployment Profiles

A deployment profile is a named bundle of the settings that decide what a client boots into: an image, an auto-install file, a SAN target, a bootloader set and an optional first-boot action. Assign a profile to a client or to a client group and every member gets those settings each time it boots. Edit the profile and the change applies to all of them immediately, because nothing is copied into the clients.

## Table of Contents

- [What a profile holds](#what-a-profile-holds)
- [Precedence](#precedence)
- [First-boot actions](#first-boot-actions)
- [Creating and assigning profiles](#creating-and-assigning-profiles)
- [Effective settings](#effective-settings)
- [Examples](#examples)
- [Via API](#via-api)

## What a profile holds

| Field | Effect at boot |
|-------|----------------|
| Image | Always offered in the client's menu, even if it is private and not in the client's or group's allowed list |
| Auto-install file | Used for images that have an unattended install, unless the client has its own file |
| SAN target and "boot from SAN by default" | Adds the SAN entry to the menu and, when ticked, makes it the entry the menu falls through to |
| Bootloader set | The iPXE binaries served to the client by proxyDHCP and TFTP |
| Public images | Show or hide public images for members, or leave the client's own setting alone |
| First-boot action | Install the profile's image unattended, or deploy a Clonezilla disk image, on the client's next boot |

## Precedence

Every field is resolved separately, from the most specific source to the least:

1. The client's own value, set on the client itself.
2. The client's profile.
3. The profile of the client's group.
4. The client group's own value, for the fields a group carries (auto-install file, bootloader set, SAN target).
5. The global default.

So a client that has a bootloader set of its own keeps it even if its profile names another one, and a group profile only matters for clients that have neither the value nor a profile of their own. A client whose public images are switched off keeps them off whatever its profile says; a client that shows public images follows the profile.

The "boot from SAN by default" flag is an exception: it is on when any of the client, its profile, its group's profile or its group has it on.

## First-boot actions

**Install the image.** The profile's image becomes the default menu entry, so the client installs it when the menu times out without anyone choosing. Once an unattended install of that image reports completion through the [install callback](auto-install.md#reporting-install-completion), the image stays in the menu but is no longer the default, so the machine falls back to the normal default on later boots. A pending next-boot action always wins over the profile default.

**Deploy the disk image.** The first time a client with this profile requests its menu, Bootimus queues a Clonezilla deploy of the chosen [disk image](disk-imaging.md) to the client's first disk and boots it straight into that job. The job is queued once per client and image; cancel it from **Disk Images** if you change your mind. Disk imaging must be ready (NFS enabled and the Clonezilla tool downloaded), otherwise the attempt is logged and the normal menu is served.

## Creating and assigning profiles

1. Open **Clients** → **Deployment Profiles** and click **Add Deployment Profile**.
2. Choose the image, auto-install file, SAN target, bootloader set and first-boot action you want; leave anything blank that the client or group should keep deciding for itself.
3. Assign it: edit a client and pick the profile under **Deployment Profile**, or edit a client group so every member inherits it.

Clients with their own profile show it as a badge in the client list. Deleting a profile clears it from every client and group that used it.

## Effective settings

Because a value can come from several places, the client editor shows an **Effective settings** block that lists what is actually in force for that machine and where each value came from: the client itself, its profile, its group's profile, its group or the default. The same information is available from `GET /api/clients/effective?mac=<MAC>`, and the menu preview reports which profile applied.

## Examples

**Zero-touch Ubuntu install.** Create a profile with the Ubuntu image, an Ubuntu cloud-init auto-install file and the first-boot action set to install. Assign it to a client group. New machines added to that group boot straight into the unattended install the next time they PXE boot and, once the install reports completion, return to the normal menu.

**Lab iSCSI boot.** Create a profile with the lab's SAN target and "boot from SAN by default" ticked, and assign it to the lab's client group. Every lab machine then falls through to its iSCSI disk after the menu timeout, while the rest of the menu remains available for rescue work.

**Golden image deploy.** Capture a reference machine with **Capture Disk**, then create a profile whose first-boot action deploys that disk image. Assign the profile to a client when it needs reimaging; the deploy is queued on its next menu request and the job appears under **Disk Images**.

## Via API

```bash
# Create a profile
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"lab-ubuntu","image_filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/lab.yaml","first_boot_action":"install"}' \
  http://localhost:8081/api/deployment-profiles

# Assign it to a client
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"profile_id":1}' "http://localhost:8081/api/clients?mac=aa:bb:cc:dd:ee:ff"

# See what is in force for that client and where each value comes from
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/effective?mac=aa:bb:cc:dd:ee:ff"
```

Client groups take `profile_id` in the same way through `PUT /api/client-groups/update?id=<id>`.
