(function () {
  "use strict";

  const STORAGE_TOKEN_KEY = "randoread.token";
  const STORAGE_EXPIRES_KEY = "randoread.expiresAt";

  const loginScreen = document.getElementById("login-screen");
  const app = document.getElementById("app");
  const dailyButton = document.getElementById("daily-button");
  const randoButton = document.getElementById("rando-button");
  const randoClippedButton = document.getElementById("rando-clipped-button");
  const clippedButton = document.getElementById("clipped-button");
  const watchingButton = document.getElementById("watching-button");
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
  // exactly what's on screen without re-fetching (and possibly picking a
  // different) Rando/Clipped note.
  let currentNote = null;

  const modeButtons = {
    daily: dailyButton,
    rando: randoButton,
    "rando-clipped": randoClippedButton,
    clipped: clippedButton,
    watching: watchingButton,
  };

  // Marks which section is active (border highlight) and reflects it in the
  // URL hash so the current view is deep-linkable/bookmarkable/shareable.
  // replaceState (not pushState) — switching sections shouldn't pile up
  // browser-history entries.
  function setActiveMode(mode) {
    for (const [key, button] of Object.entries(modeButtons)) {
      button.classList.toggle("active", key === mode);
    }
    window.history.replaceState(null, "", "#" + mode);
  }

  // Clipped/Rando Clipped titles read "Clippings / {article}" (see
  // internal/note.FormatVaultTitle). The "Clippings" segment becomes a link
  // to the full clippings list+Send-to-RM2 table; everything after it stays
  // plain text, unchanged, so it still reads as a breadcrumb back to the
  // article you came from.
  const CLIPPINGS_BREADCRUMB_PREFIX = "Clippings / ";

  function renderTitle(title) {
    noteTitle.innerHTML = "";
    if (!title.startsWith(CLIPPINGS_BREADCRUMB_PREFIX)) {
      noteTitle.textContent = title;
      return;
    }

    noteTitle.appendChild(clippingsBreadcrumbLink());
    noteTitle.appendChild(document.createTextNode(" / " + title.slice(CLIPPINGS_BREADCRUMB_PREFIX.length)));
  }

  function renderNote(data) {
    renderTitle(data.title);
    noteContent.innerHTML = data.html;
    currentNote = { path: data.path, title: data.title };
  }

  // Delegated once (rather than re-bound per video) since noteContent's
  // innerHTML — and every .video-embed inside it — is replaced wholesale on
  // every Daily/Rando/Clipped load.
  noteContent.addEventListener("click", (event) => {
    const button = event.target.closest(".video-toggle");
    if (!button) return;

    const wrapper = button.closest(".video-embed");
    const collapsed = wrapper.classList.toggle("collapsed");
    button.textContent = collapsed ? "+" : "−";
    button.setAttribute("aria-expanded", String(!collapsed));
    button.setAttribute("aria-label", collapsed ? "Expand video" : "Collapse video");
  });

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
    setActiveMode("daily");
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

  // Rando and Clipped share the same fetch/render pattern, just against
  // different endpoints and buttons. Clickable at any time — no cooldown.
  // onLoaded (optional) runs after a successful render — Watching It Later
  // uses it to kick off progress polling when the response comes back in
  // the "fetching a video" state (see loadWatching below).
  function makeFeature(button, apiPath, label, mode, onLoaded) {
    async function load() {
      setActiveMode(mode);
      noteTitle.textContent = "Loading…";
      noteContent.innerHTML = "";
      try {
        const res = await authedFetch(apiPath);
        const data = await res.json();
        if (!res.ok) {
          noteTitle.textContent = "";
          noteContent.textContent = data.error || ("Failed to load " + label.toLowerCase() + ".");
          return;
        }
        renderNote(data);
        if (onLoaded) onLoaded();
      } catch (e) {
        noteTitle.textContent = "";
        noteContent.textContent = "Failed to load " + label.toLowerCase() + ".";
      }
    }

    button.addEventListener("click", load);
    return { load };
  }

  const rando = makeFeature(randoButton, "api/rando", "Rando", "rando");
  const randoClipped = makeFeature(randoClippedButton, "api/rando-clipped", "Rando Clipped", "rando-clipped");
  const clipped = makeFeature(clippedButton, "api/clipped", "Most Recently Clipped", "clipped");

  // --- Watching It Later --------------------------------------------------

  let watchingPollTimer = null;

  function stopWatchingPoll() {
    if (!watchingPollTimer) return;
    clearInterval(watchingPollTimer);
    watchingPollTimer = null;
  }

  function showWatchingProgress(percent, label) {
    const bar = noteContent.querySelector(".watching-progress");
    if (!bar) return;
    bar.classList.remove("hidden");
    bar.querySelector(".watching-progress-bar").style.width = percent + "%";
    bar.querySelector(".watching-progress-label").textContent = label;
  }

  // Shared by the interval below and the visibilitychange listener further
  // down — Chrome/Safari throttle setInterval heavily in backgrounded tabs
  // (sometimes to once a minute or less), so a user who tabs away during a
  // real ~30-90s download and comes back could otherwise be looking at a
  // stale view until the next (delayed) tick happens to fire.
  async function checkWatchingStatus() {
    let data;
    try {
      const res = await authedFetch("api/watching/next/status");
      data = await res.json();
    } catch (e) {
      return; // transient network error while polling — keep trying
    }

    if (data.error) {
      stopWatchingPoll();
      showWatchingProgress(0, "Error: " + data.error);
      return;
    }
    if (data.noneLeft) {
      stopWatchingPoll();
      noteTitle.textContent = "";
      noteContent.innerHTML = '<div class="watching"><p>You’re all caught up 🎉</p></div>';
      return;
    }
    if (data.totalBytes) {
      const pct = Math.round(data.percent);
      showWatchingProgress(pct, pct + "%");
    }
    if (data.done) {
      stopWatchingPoll();
      watching.load();
    }
  }

  function startWatchingPoll() {
    stopWatchingPoll();
    watchingPollTimer = setInterval(checkWatchingStatus, 1000);
  }

  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && watchingPollTimer) checkWatchingStatus();
  });

  const watching = makeFeature(watchingButton, "api/watching", "Watching It Later", "watching", function () {
    if (noteContent.querySelector(".watching-next-btn")) {
      stopWatchingPoll();
    } else {
      showWatchingProgress(0, "Starting…");
      startWatchingPoll();
    }
  });

  noteContent.addEventListener("click", async (event) => {
    const nextBtn = event.target.closest(".watching-next-btn");
    if (nextBtn) {
      if (nextBtn.disabled) return;
      nextBtn.disabled = true;
      try {
        const res = await authedFetch("api/watching/next", { method: "POST" });
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          window.alert("Failed to fetch the next video: " + (data.error || res.status));
          nextBtn.disabled = false;
          return;
        }
        showWatchingProgress(0, "Starting…");
        startWatchingPoll();
      } catch (e) {
        window.alert("Failed to fetch the next video: " + e.message);
        nextBtn.disabled = false;
      }
      return;
    }

    const emojiBtn = event.target.closest(".watching-emoji-btn");
    if (!emojiBtn) return;

    const videoID = emojiBtn.getAttribute("data-video-id");
    const currentEmoji = emojiBtn.getAttribute("data-emoji") || "";
    const result = await window.WatchItLaterEmojiPicker.show(currentEmoji);
    if (result === null) return;

    try {
      const res = await authedFetch("api/watching/emoji", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ videoID: videoID, emoji: result }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        window.alert("Failed to set emoji: " + (data.error || res.status));
        return;
      }
      watching.load();
    } catch (e) {
      window.alert("Failed to set emoji: " + e.message);
    }
  });

  // Animates el's text through a growing/repeating "." ".." "..." "...."
  // "....." sequence — used on the Clippings breadcrumb link while its
  // table refetches, per Jon. Returns a stop() that restores the label.
  function animateEllipsis(el, restoreLabel) {
    const frames = [".", "..", "...", "....", "....."];
    let i = 0;
    el.textContent = frames[0];
    const interval = setInterval(() => {
      i = (i + 1) % frames.length;
      el.textContent = frames[i];
    }, 300);
    return () => {
      clearInterval(interval);
      el.textContent = restoreLabel;
    };
  }

  function renderClippingsTable(clippings) {
    noteContent.innerHTML = "";

    if (!clippings || clippings.length === 0) {
      noteContent.textContent = "No clippings in the last 3 months.";
      return;
    }

    const table = document.createElement("table");
    const thead = document.createElement("thead");
    thead.innerHTML = "<tr><th>Clipped At</th><th>Title</th><th>Action</th></tr>";
    table.appendChild(thead);

    const tbody = document.createElement("tbody");
    for (const c of clippings) {
      const tr = document.createElement("tr");

      const tdDate = document.createElement("td");
      tdDate.textContent = c.clippedAt;

      const tdTitle = document.createElement("td");
      tdTitle.textContent = c.title;

      const tdAction = document.createElement("td");
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "send-to-rm2-button";
      btn.textContent = "Send to RM2";
      btn.dataset.path = c.path;
      btn.dataset.title = c.title;
      tdAction.appendChild(btn);

      tr.append(tdDate, tdTitle, tdAction);
      tbody.appendChild(tr);
    }
    table.appendChild(tbody);

    noteContent.appendChild(table);
  }

  // Progress-feedback on the button itself (not a separate status line) —
  // per Jon, the "Send to RM2" label should reflect activity/progress.
  // Delegated (rather than bound per row) since the table is rebuilt
  // wholesale on every Clippings refetch.
  noteContent.addEventListener("click", async (event) => {
    const btn = event.target.closest(".send-to-rm2-button");
    if (!btn) return;

    const originalLabel = btn.textContent;
    btn.disabled = true;
    btn.textContent = "Sending…";

    let resultLabel;
    try {
      const res = await authedFetch("api/clippings/send-to-remarkable", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: btn.dataset.path, title: btn.dataset.title }),
      });
      const data = await res.json();
      resultLabel = res.ok ? "Sent ✓" : data.error || "Failed";
    } catch (e) {
      resultLabel = "Failed";
    }

    btn.textContent = resultLabel;
    // Revert to the original label after a few seconds so the row is
    // reusable again (e.g. resending after a transient failure), whether
    // it succeeded or not.
    setTimeout(() => {
      if (!btn.isConnected) return; // table was rebuilt/replaced meanwhile
      btn.textContent = originalLabel;
      btn.disabled = false;
    }, 3000);
  });

  function clippingsBreadcrumbLink() {
    const link = document.createElement("a");
    link.href = "#clippings-list";
    link.id = "clippings-breadcrumb-link";
    link.className = "breadcrumb-link";
    link.textContent = "Clippings";
    link.addEventListener("click", (event) => {
      event.preventDefault();
      clippingsList.load();
    });
    return link;
  }

  // Every click refetches fresh from Dropbox (no cache), per Jon — the
  // breadcrumb link animates a loading ellipsis meanwhile, then reverts to
  // just "Clippings" (table loaded below it) or an error message. No
  // trailing "/ {article}" here — we're viewing the index, not a specific
  // clipping, so that context is dropped rather than left stale.
  const clippingsList = {
    async load() {
      window.history.replaceState(null, "", "#clippings-list");

      noteTitle.innerHTML = "";
      const link = clippingsBreadcrumbLink();
      noteTitle.appendChild(link);
      const stopAnimation = animateEllipsis(link, "Clippings");

      try {
        const res = await authedFetch("api/clippings");
        const data = await res.json();
        stopAnimation();

        if (!res.ok) {
          noteContent.textContent = data.error || "Failed to load clippings.";
          return;
        }
        renderClippingsTable(data.clippings);
      } catch (e) {
        stopAnimation();
        noteContent.textContent = "Failed to load clippings.";
      }
    },
  };

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

  function loadFromHash() {
    const hash = window.location.hash.replace("#", "");
    if (hash === "rando") {
      rando.load();
    } else if (hash === "rando-clipped") {
      randoClipped.load();
    } else if (hash === "clipped") {
      clipped.load();
    } else if (hash === "clippings-list") {
      clippingsList.load();
    } else if (hash === "watching") {
      watching.load();
    } else {
      loadDaily();
    }
  }

  async function init() {
    const loggedInFromURL = await tryLoginFromURL();
    if (loggedInFromURL || storedTokenIsValid()) {
      showApp();
      loadFromHash();
      // Covers a manually edited/pasted hash on an already-open tab, not
      // just a fresh page load.
      window.addEventListener("hashchange", loadFromHash);
    } else {
      showLogin();
    }
  }

  init();
})();
