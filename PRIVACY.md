# Privacy

***English** · [繁體中文](PRIVACY.zh-TW.md)*

Updated: 2026-10-13

Taiga CLI is an independent command-line client for [Taiga 6](https://taiga.io/). It is not affiliated with the Taiga project.

## Summary

- Requests go to the Taiga server the user configures and to the addresses that server supplies. See [Network traffic](#network-traffic).
- The `taiga` binary sends no telemetry, analytics, crash reports, or update checks. The maintainer receives no data from it.
- Credentials and settings are stored locally, as listed under [Data kept on the machine](#data-kept-on-the-machine).

## Network traffic

### Taiga server requests

A command that works with Taiga sends requests to the API address in effect: the `--api-url` flag, `TAIGA_API_URL`, or the active profile's address. A request carries the content of the command: work item subjects and descriptions, comments, wiki pages, attachments, project imports, and similar data. The Taiga server's operator receives that content under the operator's own policy. The hosted Taiga's web app is at `tree.taiga.io` and its API at `api.taiga.io`; the hosted Taiga is run by the Taiga makers, not by this project.

- The access token is sent in the `Authorization` header of requests to that API.
- Password login sends the username and password to the API's `auth` endpoint. A refresh token, when present, goes to `auth/refresh` on the same API.
- The HTTP client follows redirects and stops a chain after 10 requests. On a redirect, the `Authorization` header is withheld from any other host and kept only for the same host name (any port or scheme) or a subdomain of it, including when the redirect changes HTTPS to HTTP. The client can also send the previous address, query string included, in the `Referer` header. These rules apply to every request, including attachment and CSV downloads.
- If the API answers a request that has a JSON body (login, refresh, and writes such as creating or editing work items, comments, or wiki pages) with a 307 or 308 redirect, the client re-sends that body to the redirect target, including on another domain, without the `Authorization` header. File uploads and project imports are streamed and are not re-sent.
- The client uses the proxy environment variables (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, and their lowercase forms).
- An `http://` API address is accepted and used without encryption, so the token is visible on the network path and to a proxy.
- Requests to the API and attachment downloads carry `User-Agent: taiga-cli/0.1`. Site discovery requests carry Go's default, `Go-http-client/1.1`, or `Go-http-client/2.0` over HTTP/2.
- Shell completion (`taiga __complete`, which the shell runs on Tab) sends authenticated list requests to the API for projects, work items, wiki pages, members, roles, sprints, and status, priority, severity, and type names. It reuses a cache younger than 5 minutes instead of asking again. See [Data kept on the machine](#data-kept-on-the-machine).

### Site discovery

`taiga auth login` and `taiga doctor --url` accept the URL of any page in a Taiga web app, or an API address. They fetch `conf.json` from that path and each parent path on the typed site, up the path until one names an API that answers, and request the `locales` endpoint of that API address. The address can be on another host: the hosted Taiga's `conf.json` names `api.taiga.io`. When no `conf.json` is usable, they request `locales` at the typed address and at its `api/v1/` path. A redirect to a different host or port, or from HTTPS to HTTP, is not followed; it is named in the error if discovery then fails. No credential is sent during discovery.

The login prompt defaults to the profile's saved API address, or to the hosted Taiga (`https://tree.taiga.io/`) when none is saved. Nothing is contacted until an address is accepted or entered. For the hosted Taiga this contacts `tree.taiga.io` and `api.taiga.io`.

### Attachment downloads

An attachment is downloaded from the file address the Taiga server reports for it. A relative path resolves against the API address; an address that names a host, even without a scheme, is used as given. The request carries no `Authorization` header and no cookies. A token inside the reported address is sent as part of that address. If the file address redirects, the client follows it and may send the original address, token included, as `Referer` to the redirect target.

### Connections not made

- No telemetry, analytics, crash reporting, or update check.
- No background process of the CLI's own. Requests happen only while a command runs, including a shell-completion call. With `--credential-store=keyring` (or `TAIGA_CREDENTIAL_STORE=keyring`) on Linux and no D-Bus session bus, the keyring library can run `dbus-launch`, which can leave a session bus running after the command ends.
- No connection to the `eventsUrl` that a site's `conf.json` lists for the web app's real-time updates.

## Data kept on the machine

| Data | Location | Notes |
| --- | --- | --- |
| Profiles: API URL and default project | `config.toml` in the `taiga-cli` configuration directory (`taiga-cli` under the operating system's user configuration directory) | No tokens |
| Profile and project pinned to a repository | `.git/config` of that repository (of the main repository, in a linked worktree) | Set with `taiga project use --local` or `taiga config set … --local`, and stored as `taiga.profile` and `taiga.project`. `.git/config` is not part of the committed tree |
| Credentials | Defaults: the operating system keyring on macOS and Windows; `credentials.json` in the `taiga-cli` configuration directory on Linux | `--credential-store` or `TAIGA_CREDENTIAL_STORE` selects `auto` (default), `keyring`, `file`, or `none`; `file` uses `credentials.json` on any platform. The file is not encrypted. On Unix-like systems it is created with mode 0600 in a directory created with mode 0700; on Windows it inherits the folder's permissions. `none` stores nothing. `TAIGA_TOKEN`, when set, is used in every mode and takes precedence over a stored credential. An empty `credentials.lock` file in the `taiga-cli` configuration directory (created by login, logout, and token refresh; not in `none` mode) serializes access to the credential |
| Shell completion cache | `completion-cache.json` in the `taiga-cli` configuration directory | Written when argument completion runs. Holds names Taiga returned: project slugs and names, project templates, work item references and subjects, wiki slugs, member usernames and full names, and role, sprint, status, priority, severity, and type names. Not encrypted; owner-only permissions on Unix-like systems. Entries older than 24 hours are ignored and removed the next time the cache is written |
| Downloads and exports | The path given with `--output`; without it, the attachment's file name or `<project>-<resource>.csv` in the current directory | Written to a temporary file next to the destination (owner-only permissions on Unix-like systems), then moved to the destination. The temporary file is removed when the command ends, unless the process is killed outright |
| Project dump read from standard input | A temporary file in the operating system's temporary directory | Created only when the dump is read from standard input, including for `--dry-run`. Owner-only permissions on Unix-like systems. Removed when the command ends, including on errors; a process killed outright (for example by SIGKILL or SIGHUP) leaves it behind |
| Diagnostic bundle | The path given to `taiga doctor bundle` | Written only when requested |

`taiga auth logout` removes the stored credential only. It leaves `config.toml` and the completion cache, which can be deleted by removing the `taiga-cli` configuration directory.

`--verbose` prints a line to standard error for requests to the Taiga API, except token refresh and site discovery: method, URL path without host or query string, status code (`transport-error` when no response arrived), and duration. Headers, request and response bodies, and credentials are not printed. The path of an attachment download includes the file name. The CLI writes no log file.

### Diagnostic bundle

`taiga doctor bundle <file.zip>` writes a zip file to the given path. To fill in its API, authentication, and project checks it first sends up to three read-only requests to the API address in effect (`locales`, `users/me`, and `projects/by_slug`), with the access token when one is present. The responses are not stored in the bundle.

With `--json` the command reports `"redacted": true` and `"uploaded": false`. The bundle contains the CLI version, commit, and build date; the Go version, operating system, and CPU architecture; counts of configured profiles, API URLs, and projects; presence flags for the credential, refresh token, environment token, and Git-local settings; yes/no flags for whether the current profile has an API address and a default project and whether the working directory is in a Git repository; and, for each of the four checks (configuration, API, authentication, and project), a status, an error code, whether it is retryable, and the HTTP status, with the response time in milliseconds for the API check. It also records its creation time. It contains no URLs, usernames, project names, or credentials. Automated tests check that the bundle contains none of the access token, refresh token, username, project slug, profile name, API address, site address, or the working and home directory paths.

The bundle is never uploaded. Sharing it is the user's decision.

## Installation

The install scripts (`scripts/install.sh` and `scripts/install.ps1`) contact `github.com` to resolve the latest release tag, unless a version is given, and to download one release archive and its `SHA256SUMS` file. GitHub redirects those downloads to its release-asset host, currently `release-assets.githubusercontent.com`. The scripts verify the archive against `SHA256SUMS`, copy the `taiga` binary into the install directory, and run `taiga version`. `install.sh` also runs `taiga completion` to write shell completion files into standard completion directories that already exist. `install.ps1` also adds the install directory to the user's `PATH`. The scripts contact no other host. The documented one-line install commands fetch the script from `raw.githubusercontent.com` before running it. On macOS, the first run of a quarantined binary can make the operating system check its notarization with Apple.

Installs through Homebrew, Scoop, APT, or DNF are carried out by those tools, which apply their own policies. The Homebrew formula and Scoop manifest download from `github.com`; the APT and DNF repositories are served from `koukeneko.github.io`.

## Project website

The static product page in `website/` is served by GitHub Pages at `koukeneko.github.io/taiga-cli/`, so GitHub receives each visitor's IP address and user agent. The page has no analytics, and the project runs no backend for it. It loads typefaces from Google Fonts, so a browser that opens the page also contacts Google's font servers; local fonts are used when those are unavailable. The page keeps the visitor's theme choice (`system`, `light`, or `dark`) in the browser's local storage under `taiga-website-theme`. It is not sent anywhere, and the page's own code sets no cookies.

## Contact and changes

Questions go to [GitHub issues](https://github.com/KoukeNeko/taiga-cli/issues). Changes to this document and its translation are recorded in the repository history (`git log PRIVACY.md PRIVACY.zh-TW.md`). The `Updated` date is set when the text changes.
