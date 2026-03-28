const storageKey = "ops-container-token";
const state = {
  token: localStorage.getItem(storageKey) || "",
  containers: [],
  selectedLogContainer: null,
  terminalContainer: null,
  terminalSocket: null,
  terminal: null,
  fitAddon: null,
  replayPlayer: null,
};

const elements = {
  loginView: document.getElementById("login-view"),
  dashboardView: document.getElementById("dashboard-view"),
  tokenInput: document.getElementById("token-input"),
  loginButton: document.getElementById("login-button"),
  loginMessage: document.getElementById("login-message"),
  refreshButton: document.getElementById("refresh-button"),
  logoutButton: document.getElementById("logout-button"),
  allToggle: document.getElementById("all-toggle"),
  statusMessage: document.getElementById("status-message"),
  containerTableBody: document.getElementById("container-table-body"),
  panelGrid: document.getElementById("panel-grid"),
  logPanel: document.getElementById("log-panel"),
  logTitle: document.getElementById("log-title"),
  logLines: document.getElementById("log-lines"),
  clearLogButton: document.getElementById("clear-log-button"),
  logOutput: document.getElementById("log-output"),
  terminalPanel: document.getElementById("terminal-panel"),
  terminalTitle: document.getElementById("terminal-title"),
  terminalShell: document.getElementById("terminal-shell"),
  closeTerminalButton: document.getElementById("close-terminal-button"),
  terminal: document.getElementById("terminal"),
  replayPanel: document.getElementById("replay-panel"),
  replayTitle: document.getElementById("replay-title"),
  closeReplayButton: document.getElementById("close-replay-button"),
  replayPlayer: document.getElementById("replay-player"),
};

function init() {
  bindEvents();
  if (state.token) {
    showDashboard();
    loadContainers();
  } else {
    showLogin();
  }
}

function bindEvents() {
  elements.loginButton.addEventListener("click", handleLogin);
  elements.tokenInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") handleLogin();
  });
  elements.refreshButton.addEventListener("click", loadContainers);
  elements.logoutButton.addEventListener("click", logout);
  elements.allToggle.addEventListener("change", loadContainers);
  elements.clearLogButton.addEventListener("click", () => {
    elements.logOutput.textContent = "";
    elements.logOutput.classList.add("hidden");
    state.selectedLogContainer = null;
    elements.logTitle.textContent = "容器日志";
    elements.logPanel.classList.add("hidden");
    syncPanelGrid();
  });
  elements.closeTerminalButton.addEventListener("click", closeTerminal);
  elements.closeReplayButton.addEventListener("click", closeReplay);
  window.addEventListener("resize", resizeTerminal);
}

function initTerminal() {
  if (state.terminal) return;
  state.terminal = new Terminal({
    cursorBlink: true,
    fontFamily: "IBM Plex Mono, Menlo, monospace",
    fontSize: 13,
    theme: {
      background: "#101418",
      foreground: "#d6e0ea",
    },
  });
  state.fitAddon = new FitAddon.FitAddon();
  state.terminal.loadAddon(state.fitAddon);
  state.terminal.open(elements.terminal);
  state.fitAddon.fit();
  state.terminal.onData((data) => {
    if (!state.terminalSocket || state.terminalSocket.readyState !== WebSocket.OPEN) return;
    state.terminalSocket.send(JSON.stringify({ type: "input", data }));
  });
}

function showLogin() {
  elements.loginView.classList.remove("hidden");
  elements.dashboardView.classList.add("hidden");
  elements.tokenInput.value = state.token;
}

function showDashboard() {
  elements.loginView.classList.add("hidden");
  elements.dashboardView.classList.remove("hidden");
}

async function handleLogin() {
  const token = elements.tokenInput.value.trim();
  if (!token) {
    setLoginMessage("请输入 token", true);
    return;
  }

  state.token = token;
  localStorage.setItem(storageKey, token);
  setLoginMessage("验证中...");

  try {
    await apiRequest(`/api/v1/containers?all=${elements.allToggle.checked}`);
    setLoginMessage("");
    showDashboard();
    loadContainers();
  } catch (error) {
    localStorage.removeItem(storageKey);
    setLoginMessage(error.message || "登录失败", true);
  }
}

function logout() {
  closeTerminal();
  state.token = "";
  localStorage.removeItem(storageKey);
  elements.containerTableBody.innerHTML = "";
  elements.statusMessage.textContent = "";
  showLogin();
}

async function loadContainers() {
  setStatus("加载容器列表中...");
  try {
    const response = await apiRequest(`/api/v1/containers?all=${elements.allToggle.checked}`);
    state.containers = response.data || [];
    renderTable();
    setStatus(`已加载 ${state.containers.length} 个容器`);
  } catch (error) {
    setStatus(error.message || "加载失败", true);
  }
}

function renderTable() {
  const rows = state.containers.map((container) => {
    const name = (container.Names && container.Names[0] ? container.Names[0] : "").replace(/^\//, "") || "-";
    const shortId = (container.Id || "").slice(0, 12);
    const image = container.Image || "-";
    const status = container.Status || container.State || "-";
    return `
      <tr>
        <td>${escapeHtml(name)}</td>
        <td><code>${escapeHtml(shortId)}</code></td>
        <td>${escapeHtml(image)}</td>
        <td>${escapeHtml(status)}</td>
        <td class="actions">
          <div class="action-group">
            <button class="button secondary" data-action="start" data-id="${container.Id}">启动</button>
            <button class="button danger" data-action="stop" data-id="${container.Id}">关停</button>
            <button class="button ghost" data-action="logs" data-id="${container.Id}" data-name="${escapeAttribute(name)}">查看日志</button>
            <button class="button ghost" data-action="terminal" data-id="${container.Id}" data-name="${escapeAttribute(name)}">进入终端</button>
            <button class="button ghost" data-action="replay" data-id="${container.Id}" data-name="${escapeAttribute(name)}">回放</button>
          </div>
        </td>
      </tr>
    `;
  }).join("");

  elements.containerTableBody.innerHTML = rows || `<tr><td colspan="5" class="muted">暂无容器数据</td></tr>`;
  elements.containerTableBody.querySelectorAll("button[data-action]").forEach((button) => {
    button.addEventListener("click", handleRowAction);
  });
}

async function handleRowAction(event) {
  const button = event.currentTarget;
  const { action, id, name } = button.dataset;
  if (!id) return;

  if (action === "start" || action === "stop") {
    button.disabled = true;
    try {
      await apiRequest(`/api/v1/container/${action}/${id}`, { method: "POST" });
      await loadContainers();
    } catch (error) {
      setStatus(error.message || `${action} 失败`, true);
    } finally {
      button.disabled = false;
    }
    return;
  }

  if (action === "logs") {
    await loadLogs(id, name);
    return;
  }

  if (action === "terminal") {
    openTerminal(id, name);
    return;
  }

  if (action === "replay") {
    openReplay(id, name);
  }
}

async function loadLogs(containerId, name) {
  const lines = Number(elements.logLines.value || 200);
  state.selectedLogContainer = containerId;
  hideReplay();
  elements.terminalPanel.classList.add("hidden");
  elements.terminal.classList.add("hidden");
  if (state.terminalSocket) {
    state.terminalSocket.close();
    state.terminalSocket = null;
  }
  elements.logPanel.classList.remove("hidden");
  syncPanelGrid();
  elements.logTitle.textContent = `容器日志: ${name || containerId.slice(0, 12)}`;
  elements.logOutput.classList.remove("hidden");
  elements.logOutput.textContent = "日志加载中...";
  try {
    const response = await apiRequest(`/api/v1/container/log/${containerId}?lines=${lines}`, { method: "POST" });
    elements.logOutput.textContent = (response.data || []).join("\n") || "没有日志输出";
    elements.logOutput.scrollTop = elements.logOutput.scrollHeight;
  } catch (error) {
    elements.logOutput.textContent = error.message || "日志加载失败";
  }
}

function openTerminal(containerId, name) {
  closeTerminal(false);
  hideReplay();
  initTerminal();
  state.terminalContainer = containerId;
  elements.logPanel.classList.add("hidden");
  elements.logOutput.classList.add("hidden");
  elements.terminalPanel.classList.remove("hidden");
  syncPanelGrid();
  elements.terminalTitle.textContent = `容器终端: ${name || containerId.slice(0, 12)}`;
  elements.terminal.classList.remove("hidden");
  state.terminal.clear();
  state.terminal.writeln(`Connecting to ${name || containerId} ...`);

  const shell = encodeURIComponent(elements.terminalShell.value.trim() || "/bin/sh");
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const socketUrl = `${protocol}//${window.location.host}/api/v1/container/exec/${containerId}?shell=${shell}&cols=${state.fitAddon.proposeDimensions()?.cols || 120}&rows=${state.fitAddon.proposeDimensions()?.rows || 40}&token=${encodeURIComponent(state.token)}`;
  const socket = new WebSocket(socketUrl);
  state.terminalSocket = socket;

  socket.onopen = () => {
    resizeTerminal();
  };
  socket.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data);
      if (message.type === "output") {
        state.terminal.write(message.data);
      } else if (message.type === "error") {
        state.terminal.writeln(`\r\n[error] ${message.data}`);
      } else if (message.type === "exit") {
        state.terminal.writeln(`\r\n[process exited with code ${message.code}]`);
      }
    } catch {
      state.terminal.write(event.data);
    }
  };
  socket.onerror = () => {
    state.terminal.writeln("\r\n[terminal connection error]");
  };
  socket.onclose = () => {
    state.terminal.writeln("\r\n[terminal disconnected]");
  };
}

function closeTerminal(clearTitle = true) {
  if (state.terminalSocket) {
    state.terminalSocket.close();
    state.terminalSocket = null;
  }
  if (clearTitle) {
    elements.terminalTitle.textContent = "容器终端";
    elements.terminal.classList.add("hidden");
    elements.terminalPanel.classList.add("hidden");
    syncPanelGrid();
    if (state.terminal) {
      state.terminal.clear();
    }
  }
}

async function openReplay(containerId, name) {
  closeTerminal();
  elements.logPanel.classList.add("hidden");
  elements.replayPanel.classList.remove("hidden");
  elements.replayTitle.textContent = `终端回放: ${name || containerId.slice(0, 12)}`;
  elements.replayPlayer.classList.add("hidden");
  elements.replayPlayer.textContent = "正在加载回放...";
  elements.replayPlayer.classList.remove("hidden");
  elements.replayPlayer.innerHTML = "正在加载回放...";
  syncPanelGrid();

  try {
    const response = await apiRequest(`/api/v1/container/recordings/${containerId}`);
    const recordings = response.data || [];
    if (!recordings.length) {
      elements.replayPlayer.textContent = "当前容器还没有录制记录，先进入一次终端执行命令后再回放。";
      return;
    }

    const latest = recordings[0];
    elements.replayPlayer.innerHTML = "";
    state.replayPlayer = AsciinemaPlayer.create(latest.url, elements.replayPlayer, {
      autoPlay: true,
      fit: "width",
      preload: true,
      terminalFontSize: "13px",
      theme: "asciinema",
    });
  } catch (error) {
    elements.replayPlayer.textContent = error.message || "加载回放失败";
  }
}

function closeReplay() {
  hideReplay();
  syncPanelGrid();
}

function hideReplay() {
  elements.replayPanel.classList.add("hidden");
  elements.replayTitle.textContent = "终端回放";
  elements.replayPlayer.classList.add("hidden");
  elements.replayPlayer.innerHTML = "";
  state.replayPlayer = null;
}

function syncPanelGrid() {
  const showGrid =
    !elements.logPanel.classList.contains("hidden") ||
    !elements.terminalPanel.classList.contains("hidden") ||
    !elements.replayPanel.classList.contains("hidden");
  elements.panelGrid.classList.toggle("hidden", !showGrid);
}

function resizeTerminal() {
  if (!state.fitAddon || !state.terminal || elements.terminal.classList.contains("hidden")) return;
  state.fitAddon.fit();
  if (!state.terminalSocket || state.terminalSocket.readyState !== WebSocket.OPEN) return;
  const dims = state.fitAddon.proposeDimensions();
  if (!dims) return;
  state.terminalSocket.send(JSON.stringify({ type: "resize", cols: dims.cols, rows: dims.rows }));
}

async function apiRequest(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: {
      Authorization: state.token,
      ...(options.headers || {}),
    },
  });

  let payload = null;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  }

  if (!response.ok) {
    throw new Error(payload?.msg || `request failed: ${response.status}`);
  }

  if (payload && payload.code && payload.code !== 200) {
    throw new Error(payload.msg || "request failed");
  }

  return payload;
}

function setLoginMessage(message, isError = false) {
  elements.loginMessage.textContent = message;
  elements.loginMessage.className = isError ? "message error" : "message";
}

function setStatus(message, isError = false) {
  elements.statusMessage.textContent = message;
  elements.statusMessage.className = isError ? "muted error" : "muted";
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttribute(value) {
  return escapeHtml(value).replaceAll("`", "&#96;");
}

init();
