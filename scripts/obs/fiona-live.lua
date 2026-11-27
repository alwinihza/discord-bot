-- Tells the Fiona Discord bot when streaming starts and stops.
-- Install: OBS > Tools > Scripts > + > pick this file, then fill in the URL and secret.

local ffi = require("ffi")

local url, secret = "", ""

local function windows_quote(s)
  return '"' .. s:gsub('"', '\\"') .. '"'
end

local function shell_quote(s)
  return "'" .. s:gsub("'", "'\\''") .. "'"
end

local spawn
if ffi.os == "Windows" then
  ffi.cdef [[
    typedef struct {
      uint32_t cb; void *lpReserved; void *lpDesktop; void *lpTitle;
      uint32_t dwX, dwY, dwXSize, dwYSize, dwXCountChars, dwYCountChars, dwFillAttribute, dwFlags;
      uint16_t wShowWindow, cbReserved2; void *lpReserved2;
      void *hStdInput, *hStdOutput, *hStdError;
    } STARTUPINFOA;
    typedef struct { void *hProcess, *hThread; uint32_t dwProcessId, dwThreadId; } PROCESS_INFORMATION;
    int CreateProcessA(const char *app, char *cmd, void *pa, void *ta, int inherit, uint32_t flags,
                       void *env, const char *dir, STARTUPINFOA *si, PROCESS_INFORMATION *pi);
    int CloseHandle(void *h);
    uint32_t GetLastError(void);
  ]]
  local kernel32 = ffi.load("kernel32")
  local CREATE_NO_WINDOW = 0x08000000

  -- os.execute would flash a console window and block OBS until curl exits.
  spawn = function(args)
    local quoted = {}
    for i, a in ipairs(args) do quoted[i] = windows_quote(a) end
    local line = table.concat(quoted, " ")
    local cmd = ffi.new("char[?]", #line + 1)
    ffi.copy(cmd, line)
    local si = ffi.new("STARTUPINFOA")
    si.cb = ffi.sizeof(si)
    local pi = ffi.new("PROCESS_INFORMATION")
    if kernel32.CreateProcessA(nil, cmd, nil, nil, 0, CREATE_NO_WINDOW, nil, nil, si, pi) == 0 then
      return false, "CreateProcess failed, error " .. kernel32.GetLastError()
    end
    kernel32.CloseHandle(pi.hThread)
    kernel32.CloseHandle(pi.hProcess)
    return true
  end
else
  spawn = function(args)
    local quoted = {}
    for i, a in ipairs(args) do quoted[i] = shell_quote(a) end
    os.execute(table.concat(quoted, " ") .. " >/dev/null 2>&1 &")
    return true
  end
end

-- Global so it can be tested outside OBS.
function send_event(to, key, event)
  if to == "" or key == "" then
    return false, "set the webhook URL and secret in Tools > Scripts"
  end
  return spawn({
    ffi.os == "Windows" and "curl.exe" or "curl",
    "-sS", "--max-time", "10", "--retry", "3",
    "-X", "POST",
    "-H", "Content-Type: application/json",
    "-H", "Authorization: Bearer " .. key,
    "-d", '{"event":"' .. event .. '"}',
    to,
  })
end

if not obslua then return end
local obs = obslua

local function notify(event)
  local ok, err = send_event(url, secret, event)
  if ok then
    obs.script_log(obs.LOG_INFO, "sent " .. event)
  else
    obs.script_log(obs.LOG_WARNING, "could not send " .. event .. ": " .. err)
  end
end

local function on_event(e)
  if e == obs.OBS_FRONTEND_EVENT_STREAMING_STARTED then
    notify("started")
  elseif e == obs.OBS_FRONTEND_EVENT_STREAMING_STOPPED then
    notify("stopped")
  end
end

function script_description()
  return "Tells the Fiona Discord bot when you start and stop streaming, so it can post the stream in Discord."
end

function script_properties()
  local p = obs.obs_properties_create()
  obs.obs_properties_add_text(p, "url", "Webhook URL", obs.OBS_TEXT_DEFAULT)
  obs.obs_properties_add_text(p, "secret", "Secret", obs.OBS_TEXT_PASSWORD)
  return p
end

function script_defaults(settings)
  obs.obs_data_set_default_string(settings, "url", "https://hooks.alwinihza.com/hooks/obs")
end

function script_update(settings)
  url = obs.obs_data_get_string(settings, "url")
  secret = obs.obs_data_get_string(settings, "secret")
end

function script_load(settings)
  obs.obs_frontend_add_event_callback(on_event)
end
