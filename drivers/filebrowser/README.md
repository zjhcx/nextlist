# FileBrowser

This driver supports FileBrowser Quantum **2.x** through its REST API.

Configure a storage with driver `FileBrowser`:

| Field | Value |
| --- | --- |
| `address` | Site URL, including the reverse proxy base path if present, e.g. `https://files.example.com/filebrowser` |
| `auth` | API token with access to the selected source |
| `source` | Source name configured in FileBrowser Quantum |
| `root_folder_path` | Directory within the source, default `/` |
| `thumbnail` | Show available Quantum thumbnails, default `true` |
| `checksum` | Compute `md5`, `sha1` or `sha256` during single-object lookup; default `none` |
| `enable_direct_upload` | Enable browser `HttpDirect` uploads; default `false` |
| `native_archive` | Enable compatible server-side extraction; default `true` |

> [!WARNING]
> The configured API token is embedded in download links, thumbnail URLs and (if enabled) direct upload URLs, so **anyone who can list or download files from this storage can obtain the token** and use the FileBrowser API directly, bypassing OpenList permissions, meta passwords and `root_folder_path`. Only use this driver with a FileBrowser source that contains no sensitive files, and prefer a token scoped to that source with the minimum required permissions.

The driver sends the token as the `auth` query parameter for API requests and download links. Replace the configured token when it expires or is revoked. Operations follow the token's permissions and the source's access rules.

Supported operations: directory listing, download, upload (including overwrite), directory creation, copy, move, rename and delete. Download URLs use `/api/resources/download` and include `source`, `file` and `auth`.

OpenList multipart upload Sessions are supported through the standard `op.Put` pipeline. The driver consumes the Session's non-seekable stream directly, reports progress, honors cancellation and does not retry or buffer the upload itself. Permission failures map to OpenList's permanent upload error classification. The `X-File-Total-Size` header enables Quantum's staged upload and length verification before replacing the destination.

API reference: https://filebrowserquantum.com/en/docs/reference/api/

Advanced capabilities:

- Single-object metadata lookup avoids listing the parent directory; available checksums populate OpenList's hash metadata. Explicitly enabling checksum calculation can require reading the entire remote file.
- Thumbnails use Quantum's preview endpoint when the server reports a preview is available.
- Browser direct upload uses a single POST with a declared total size. Quantum must allow the OpenList frontend origin, POST requests and the `Content-Type` and `X-File-Total-Size` headers through CORS. Enabling this option gives upload clients the configured API token in the upload URL; it is not a token scoped to one file.
- Native extraction supports complete `.zip`, `.tar.gz` and `.tgz` archives with overwrite enabled, optionally into a new folder. Passwords, inner-path extraction, other formats and no-overwrite requests fall back to OpenList's internal archive tools. Native extraction requires a Quantum version exposing `/api/resources/unarchive` and appropriate source permissions.
- Range downloads use Quantum's download endpoint and OpenList's proxy range setting.

Remote capacity reporting, hash-based rapid upload and native archive browsing are not exposed by the APIs used here. Archive browsing and individual entry extraction continue to use OpenList's built-in tools.
