(function () {
  "use strict";

  const STORAGE_TOKEN_KEY = "randoread.token";
  const STORAGE_EXPIRES_KEY = "randoread.expiresAt";

  const loginScreen = document.getElementById("login-screen");
  const app = document.getElementById("app");
  const dailyButton = document.getElementById("daily-button");
  const randoButton = document.getElementById("rando-button");
  const clippedButton = document.getElementById("clipped-button");
  const noteTitle = document.getElementById("note-title");
  const noteContent = document.getElementById("note-content");
  const menuButton = document.getElementById("menu-button");
  const menuPanel = document.getElementById("menu-panel");
  const dropboxStatus = document.getElementById("dropbox-status");
  const dropboxConnectBtn = document.getElementById("dropbox-connect-btn");
  const dropboxDisconnectBtn = document.getElementById("dropbox-disconnect-btn");
  const emailButton = document.getElementById("email-button");
  const emailStatus = document.getElementById("email-status");

  // The currently displayed note — needed so "Email this note" can send
  // exactly what's on screen without re-picking a new Rando/Clipped note
  // (which would also burn its 24h cooldown).
  let currentNote = null;

  function renderNote(data) {
    noteTitle.textContent = data.title;
    noteContent.innerHTML = data.html;
    currentNote = { path: data.path, title: data.title };
  }

  function storedToken() {
    return localStorage.getItem(STORAGE_TOKEN_KEY);
  }

  function authedFetch(path, options) {
    options = options || {};
    options.headers = Object.assign({}, options.headers, {
      "X-Auth-Token": storedToken(),
    });
    return fetch(path, options);
  }

  async function refreshDropboxStatus() {
    try {
      const res = await authedFetch("api/dropbox/status");
      const data = await res.json();
      dropboxStatus.textContent = "Dropbox: " + (data.connected ? "connected" : "not connected");
      dropboxConnectBtn.classList.toggle("hidden", data.connected);
      dropboxDisconnectBtn.classList.toggle("hidden", !data.connected);
    } catch (e) {
      dropboxStatus.textContent = "Dropbox: status unavailable";
    }
  }

  menuButton.addEventListener("click", () => {
    menuPanel.classList.toggle("hidden");
    if (!menuPanel.classList.contains("hidden")) {
      refreshDropboxStatus();
    }
  });

  dropboxConnectBtn.addEventListener("click", () => {
    // Full-page navigation (OAuth redirect flow) — can't set a custom
    // header, so the token travels as a query param here (RequireToken
    // accepts either). See handlers/auth.go for the server-side fallback.
    window.location.href = "api/dropbox/auth?token=" + encodeURIComponent(storedToken());
  });

  dropboxDisconnectBtn.addEventListener("click", async () => {
    await authedFetch("api/dropbox/disconnect", { method: "POST" });
    refreshDropboxStatus();
  });

  emailButton.addEventListener("click", async () => {
    if (!currentNote) {
      emailStatus.textContent = "Load a note first.";
      return;
    }
    emailStatus.textContent = "Sending…";
    try {
      const res = await authedFetch("api/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(currentNote),
      });
      const data = await res.json();
      emailStatus.textContent = res.ok ? "Sent!" : data.error || "Failed to send.";
    } catch (e) {
      emailStatus.textContent = "Failed to send.";
    }
  });

  async function loadDaily() {
    noteTitle.textContent = "Loading…";
    noteContent.innerHTML = "";
    try {
      const res = await authedFetch("api/daily");
      const data = await res.json();
      if (!res.ok) {
        noteTitle.textContent = "";
        noteContent.textContent = data.error || "Failed to load today's daily note.";
        return;
      }
      renderNote(data);
    } catch (e) {
      noteTitle.textContent = "";
      noteContent.textContent = "Failed to load today's daily note.";
    }
  }

  dailyButton.addEventListener("click", loadDaily);

  function formatRetryAfter(seconds) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return "Available in " + hours + "h " + minutes + "m";
  }

  // Rando and Clipped share the same 24h-gated fetch/render/disable pattern,
  // just against different endpoints and buttons.
  function makeGatedFeature(button, apiPath, statusPath, label) {
    async function refreshStatus() {
      try {
        const res = await authedFetch(statusPath);
        const data = await res.json();
        button.disabled = data.onCooldown;
        button.title = data.onCooldown ? formatRetryAfter(data.retryAfterSeconds) : "";
      } catch (e) {
        // leave the button as-is; load() will surface any real error
      }
    }

    async function load() {
      noteTitle.textContent = "Loading…";
      noteContent.innerHTML = "";
      try {
        const res = await authedFetch(apiPath);
        const data = await res.json();
        if (res.status === 429) {
          noteTitle.textContent = "";
          noteContent.textContent = label + " is on cooldown — " + formatRetryAfter(data.retryAfterSeconds).toLowerCase();
          refreshStatus();
          return;
        }
        if (!res.ok) {
          noteTitle.textContent = "";
          noteContent.textContent = data.error || ("Failed to load " + label.toLowerCase() + ".");
          return;
        }
        renderNote(data);
        refreshStatus();
      } catch (e) {
        noteTitle.textContent = "";
        noteContent.textContent = "Failed to load " + label.toLowerCase() + ".";
      }
    }

    button.addEventListener("click", load);
    return { refreshStatus };
  }

  const rando = makeGatedFeature(randoButton, "api/rando", "api/rando/status", "Rando");
  const clipped = makeGatedFeature(clippedButton, "api/clipped", "api/clipped/status", "Clipped");

  function storedTokenIsValid() {
    const token = localStorage.getItem(STORAGE_TOKEN_KEY);
    const expiresAt = localStorage.getItem(STORAGE_EXPIRES_KEY);
    if (!token || !expiresAt) return false;
    return new Date(expiresAt).getTime() > Date.now();
  }

  function showApp() {
    loginScreen.classList.add("hidden");
    app.classList.remove("hidden");
  }

  function showLogin() {
    app.classList.add("hidden");
    loginScreen.classList.remove("hidden");
  }

  async function tryLoginFromURL() {
    const params = new URLSearchParams(window.location.search);
    const token = params.get("token");
    if (!token) return false;

    // Relative to <base href="/randoread/"> in index.html — see the comment
    // there for why this can't be an absolute "/api/auth" path.
    const res = await fetch("api/auth?token=" + encodeURIComponent(token));
    const data = await res.json();
    if (!res.ok || !data.valid) return false;

    localStorage.setItem(STORAGE_TOKEN_KEY, token);
    localStorage.setItem(STORAGE_EXPIRES_KEY, data.expiresAt);

    // Strip the token from the URL so it doesn't linger in history/referrers.
    const url = new URL(window.location.href);
    url.searchParams.delete("token");
    window.history.replaceState({}, "", url.toString());

    return true;
  }

  async function init() {
    const loggedInFromURL = await tryLoginFromURL();
    if (loggedInFromURL || storedTokenIsValid()) {
      showApp();
      loadDaily();
      rando.refreshStatus();
      clipped.refreshStatus();
    } else {
      showLogin();
    }
  }

  init();
})();
