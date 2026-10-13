# 隱私

*[English](PRIVACY.md) · **繁體中文***

更新日期：2026-10-13

Taiga CLI 是 [Taiga 6](https://taiga.io/) 的獨立命令列客戶端，與 Taiga 專案沒有隸屬關係。

## 摘要

- 請求會送到使用者設定的 Taiga 伺服器，以及該伺服器提供的位址。見[網路流量](#網路流量)。
- `taiga` 執行檔不傳送遙測、使用分析、當機回報或更新檢查，維護者不會從它收到任何資料。
- 憑證與設定儲存在本機，位置見[留在本機的資料](#留在本機的資料)。

## 網路流量

### Taiga 伺服器請求

與 Taiga 互動的指令，會把請求送到目前生效的 API 位址：`--api-url` 旗標、`TAIGA_API_URL`，或目前 profile 的位址。請求帶的是指令本身的內容：工作項目的標題與描述、留言、wiki 頁面、附件、專案匯入等資料。這些內容由 Taiga 伺服器的營運者接收，並依營運者自己的政策處理。託管版 Taiga 的網頁應用在 `tree.taiga.io`，API 在 `api.taiga.io`；託管版 Taiga 由 Taiga 的開發團隊營運，不是本專案。

- 存取 token 會放在送往該 API 的請求的 `Authorization` 標頭。
- 密碼登入會把使用者名稱與密碼送到 API 的 `auth` 端點。有 refresh token 時，會送到同一個 API 的 `auth/refresh`。
- HTTP 客戶端會跟隨轉址，並在一連串轉址達到 10 次請求時停止。轉址時，`Authorization` 標頭不會送往任何其他主機，只有在相同的主機名稱（任何連接埠或協定）或其子網域時才會保留，包括轉址把 HTTPS 降為 HTTP 的情況。客戶端也可能在 `Referer` 標頭中帶上前一個位址，含查詢字串。這些規則適用於每一個請求，包括附件與 CSV 下載。
- 若 API 以 307 或 308 轉址回應帶有 JSON 內容的請求（登入、refresh，以及建立或編輯工作項目、留言、wiki 頁面等寫入），客戶端會把該內容重新送到轉址目標，包括不同網域，但不帶 `Authorization` 標頭。檔案上傳與專案匯入以串流方式傳送，不會重新送出。
- 客戶端使用代理伺服器的環境變數（`HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY` 及其小寫形式）。
- 接受 `http://` 的 API 位址，並以未加密的方式使用，因此 token 在網路路徑上與代理伺服器都看得到。
- 送往 API 的請求與附件下載帶有 `User-Agent: taiga-cli/0.1`。站台探測的請求帶的是 Go 的預設值 `Go-http-client/1.1`，在 HTTP/2 下則是 `Go-http-client/2.0`。
- Shell completion（`taiga __complete`，由 shell 在按下 Tab 時執行）會向 API 送出需驗證的列表請求，涵蓋專案、工作項目、wiki 頁面、成員、角色、sprint，以及狀態、優先順序、嚴重程度與類型的名稱。快取不到 5 分鐘時會重複使用，不再重新詢問。見[留在本機的資料](#留在本機的資料)。

### 站台探測

`taiga auth login` 與 `taiga doctor --url` 接受 Taiga 網頁應用中任何一頁的網址，或 API 位址。它們會從該路徑與輸入站台上的每一層上層路徑取得 `conf.json`，一路往上直到有一個指定了會回應的 API，再向該 API 位址請求 `locales` 端點。該位址可以在另一個主機：託管版 Taiga 的 `conf.json` 指定的是 `api.taiga.io`。沒有可用的 `conf.json` 時，會在輸入的位址與其 `api/v1/` 路徑請求 `locales`。轉址到不同的主機或連接埠，或從 HTTPS 轉為 HTTP 時，不會跟隨；若探測因此失敗，錯誤訊息中會指出該轉址。探測過程不送出任何憑證。

登入提示預設為 profile 已儲存的 API 位址；沒有儲存時，預設為託管版 Taiga（`https://tree.taiga.io/`）。在接受或輸入位址之前不會連線。對託管版 Taiga 而言，會連線到 `tree.taiga.io` 與 `api.taiga.io`。

### 附件下載

附件從 Taiga 伺服器回報的檔案位址下載。相對路徑會以 API 位址解析；指名了主機的位址，即使沒有協定，也照原樣使用。下載請求不帶 `Authorization` 標頭，也不帶 cookie。伺服器回報的位址中若含有 token，會隨該位址一併送出。若檔案位址轉址，客戶端會跟隨，並可能把原始位址（含 token）以 `Referer` 送給轉址目標。

### CLI 不會建立的連線

- 沒有遙測、使用分析、當機回報或更新檢查。
- CLI 本身沒有背景行程。只有在指令執行期間才會發出請求，包括一次 shell completion 呼叫。在 Linux 上使用 `--credential-store=keyring`（或 `TAIGA_CREDENTIAL_STORE=keyring`）且沒有 D-Bus session bus 時，keyring 函式庫可能執行 `dbus-launch`，它可能在指令結束後留下仍在執行的 session bus。
- 不會連線到站台 `conf.json` 為網頁應用即時更新所列出的 `eventsUrl`。

## 留在本機的資料

| 資料 | 位置 | 說明 |
| --- | --- | --- |
| Profile：API URL 與預設專案 | `taiga-cli` 設定目錄中的 `config.toml`（作業系統使用者設定目錄下的 `taiga-cli`） | 不含 token |
| 綁定到某個 repository 的 profile 與專案 | 該 repository 的 `.git/config`（在 linked worktree 中則是主要 repository 的 `.git/config`） | 以 `taiga project use --local` 或 `taiga config set … --local` 設定，儲存為 `taiga.profile` 與 `taiga.project`。`.git/config` 不屬於會被 commit 的檔案樹 |
| 憑證 | 預設：macOS 與 Windows 為作業系統的 keyring；Linux 為 `taiga-cli` 設定目錄中的 `credentials.json` | `--credential-store` 或 `TAIGA_CREDENTIAL_STORE` 可選 `auto`（預設）、`keyring`、`file` 或 `none`；`file` 在任何平台都使用 `credentials.json`。檔案未加密。在類 Unix 系統上，檔案以 0600 權限建立於以 0700 權限建立的目錄中；在 Windows 上沿用所在資料夾的權限。`none` 不儲存任何東西。設定了 `TAIGA_TOKEN` 時，所有模式都使用它，且優先於已儲存的憑證。`taiga-cli` 設定目錄中有一個空的 `credentials.lock` 檔案（由登入、登出與 token 更新建立；`none` 模式不會建立），用來序列化對憑證的存取 |
| Shell completion 快取 | `taiga-cli` 設定目錄中的 `completion-cache.json` | 在執行參數補全時寫入。內容是 Taiga 回傳的名稱：專案 slug 與名稱、專案範本、工作項目編號與標題、wiki slug、成員的使用者名稱與全名，以及角色、sprint、狀態、優先順序、嚴重程度與類型的名稱。未加密；在類 Unix 系統上僅限擁有者存取。超過 24 小時的項目會被忽略，並在下次寫入快取時移除 |
| 下載與匯出 | 以 `--output` 指定的路徑；未指定時，為附件的檔名，或目前目錄中的 `<project>-<resource>.csv` | 先寫入目的地旁的暫存檔（在類 Unix 系統上僅限擁有者存取），再移到目的地。指令結束時會移除暫存檔，除非行程被強制終止 |
| 從標準輸入讀入的專案 dump | 作業系統暫存目錄中的暫存檔 | 只在從標準輸入讀入 dump 時建立，`--dry-run` 也一樣。在類 Unix 系統上僅限擁有者存取。指令結束時移除，包括發生錯誤時；行程被強制終止（例如 SIGKILL 或 SIGHUP）時會留下 |
| 診斷包 | 交給 `taiga doctor bundle` 的路徑 | 只在要求時寫入 |

`taiga auth logout` 只會移除已儲存的憑證。`config.toml` 與 completion 快取會留下，可以刪除 `taiga-cli` 設定目錄來移除。

`--verbose` 會把送往 Taiga API 的請求印成一行到標準錯誤，但 token 更新與站台探測除外：方法、不含主機與查詢字串的 URL 路徑、狀態碼（沒有收到回應時為 `transport-error`）與耗時。不會印出標頭、請求與回應內容或憑證。附件下載的路徑含有檔名。CLI 不寫入任何記錄檔。

### 診斷包

`taiga doctor bundle <file.zip>` 會把 zip 檔寫到指定的路徑。為了填入 API、身分驗證與專案的檢查結果，它會先向目前生效的 API 位址送出最多三個唯讀請求（`locales`、`users/me` 與 `projects/by_slug`），有存取 token 時會一併帶上。回應不會存進診斷包。

加上 `--json` 時，指令會回報 `"redacted": true` 與 `"uploaded": false`。診斷包的內容有：CLI 版本、commit 與建置日期；Go 版本、作業系統與 CPU 架構；已設定的 profile、API URL 與專案的數量；憑證、refresh token、環境變數 token 與 Git 本機設定是否存在的旗標；目前 profile 是否有 API 位址與預設專案、工作目錄是否在 Git repository 中的是／否旗標；以及四項檢查（設定、API、身分驗證與專案）各自的狀態、錯誤代碼、是否可重試與 HTTP 狀態，API 檢查另有以毫秒計的回應時間。診斷包也會記錄建立時間。它不含 URL、使用者名稱、專案名稱或憑證。自動化測試會檢查診斷包不含存取 token、refresh token、使用者名稱、專案 slug、profile 名稱、API 位址、站台位址，以及工作目錄與家目錄的路徑。

診斷包不會被上傳。是否分享由使用者決定。

## 安裝

安裝腳本（`scripts/install.sh` 與 `scripts/install.ps1`）會連線到 `github.com`，在未指定版本時解析最新的 release tag，並下載一個 release archive 與其 `SHA256SUMS` 檔案。GitHub 會把這些下載轉址到它的 release asset 主機，目前是 `release-assets.githubusercontent.com`。腳本會用 `SHA256SUMS` 驗證 archive，把 `taiga` 執行檔複製到安裝目錄，並執行 `taiga version`。`install.sh` 還會執行 `taiga completion`，把 shell completion 檔案寫入已存在的標準 completion 目錄。`install.ps1` 還會把安裝目錄加入使用者的 `PATH`。腳本不會連線到其他主機。安裝說明中的單行安裝指令會在執行腳本之前，先從 `raw.githubusercontent.com` 取得腳本。在 macOS 上，第一次執行被隔離（quarantine）的執行檔時，作業系統可能會向 Apple 檢查它的 notarization。

透過 Homebrew、Scoop、APT 或 DNF 安裝時，由這些工具執行，並適用它們各自的政策。Homebrew formula 與 Scoop manifest 從 `github.com` 下載；APT 與 DNF 的套件庫由 `koukeneko.github.io` 提供。

## 專案網站

`website/` 中的靜態產品頁由 GitHub Pages 提供，位於 `koukeneko.github.io/taiga-cli/`，因此 GitHub 會收到每位訪客的 IP 位址與 user agent。該頁沒有使用分析，本專案也沒有為它執行後端。它從 Google Fonts 載入字型，因此開啟該頁的瀏覽器也會連線到 Google 的字型伺服器；無法連線時改用本機字型。該頁會把訪客選擇的佈景主題（`system`、`light` 或 `dark`）以 `taiga-website-theme` 為名存在瀏覽器的 local storage 中。這個值不會傳到任何地方，頁面本身的程式碼也不設定 cookie。

## 聯絡與異動

問題請到 [GitHub issues](https://github.com/KoukeNeko/taiga-cli/issues)。本頁與其英文版的異動記錄在 repository 的歷史中（`git log PRIVACY.md PRIVACY.zh-TW.md`）。內容有異動時，「更新日期」會一併設定。
