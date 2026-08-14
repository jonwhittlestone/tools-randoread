// Floating "Send to oh-two" journal input — see main-randoread.md 05.05 /
// the 26-nanoclaw vault project's main.md §05.05. Self-contained (own
// authedFetch), same split-out-of-app.js shape as watching-notes.js.
//
// Visible only on the Daily note page — the journal is inherently a Daily
// concept (it only ever writes to *today's* note), so showing it while
// browsing Rando/Clipped/Watching would just be confusing. Tracked via a
// MutationObserver on #daily-button's "active" class rather than a custom
// event, since app.js's setActiveMode toggles that class on every section
// switch (click, hash routing, and initial page load all funnel through
// it) and isn't otherwise instrumented for other scripts to hook into.
//
// Availability: NanoClaw runs on a different host from randoread, reachable
// only while that host is on the Tailscale network (see main.md §05.05.02
// "Security"). The bar is hidden entirely, not just disabled, whenever
// GET /api/journal/status reports unavailable — checked on load and
// rechecked periodically, since reachability can change after load (dev
// laptop sleeping/waking, tailnet dropping).
//
// Both conditions (Daily active AND available) gate the same "hidden"
// class — see updateBarVisibility.
(function () {
  "use strict";

  var STORAGE_TOKEN_KEY = "randoread.token";
  var AVAILABILITY_RECHECK_MS = 60000;

  var dailyButton = document.getElementById("daily-button");
  if (!dailyButton) return;

  function authedFetch(path, options) {
    options = options || {};
    options.headers = Object.assign({}, options.headers, {
      "X-Auth-Token": localStorage.getItem(STORAGE_TOKEN_KEY),
    });
    return fetch(path, options);
  }

  function fetchJSON(path, options) {
    return authedFetch(path, options).then(function (res) {
      return res.json().then(function (data) {
        return { ok: res.ok, data: data };
      });
    });
  }

  // --- Floating bar ---
  var bar = document.createElement("div");
  bar.className = "journal-bar hidden";
  bar.innerHTML =
    '<button type="button" class="journal-attach-btn" disabled title="Image attachments coming soon">📎</button>' +
    '<input type="text" class="journal-input" placeholder="Tell oh-two something to note down…">' +
    '<button type="button" class="journal-send-btn">Send to oh-two</button>' +
    '<span class="journal-status"></span>';
  document.body.appendChild(bar);

  var input = bar.querySelector(".journal-input");
  var sendBtn = bar.querySelector(".journal-send-btn");
  var status = bar.querySelector(".journal-status");

  // --- Confirm modal (same overlay/modal shape as emoji-picker.js) ---
  var overlay = document.createElement("div");
  overlay.className = "journal-modal-overlay";
  overlay.innerHTML =
    '<div class="journal-modal">' +
    '<p class="journal-modal-reply"></p>' +
    '<pre class="journal-modal-insertion"></pre>' +
    '<div class="journal-modal-actions">' +
    '<button type="button" class="journal-modal-btn cancel">Don’t update and close</button>' +
    '<button type="button" class="journal-modal-btn ok">OK</button>' +
    "</div>" +
    "</div>";
  document.body.appendChild(overlay);

  var modalReply = overlay.querySelector(".journal-modal-reply");
  var modalInsertion = overlay.querySelector(".journal-modal-insertion");
  var modalCancelBtn = overlay.querySelector(".cancel");
  var modalOkBtn = overlay.querySelector(".ok");

  var pendingDraft = null; // {heading, insertionMarkdown, reply} awaiting confirm

  function openModal(draft) {
    pendingDraft = draft;
    modalReply.textContent = draft.reply;
    modalInsertion.textContent = draft.insertionMarkdown;
    overlay.classList.add("visible");
  }

  function closeModal() {
    overlay.classList.remove("visible");
    pendingDraft = null;
  }

  overlay.addEventListener("click", function (e) {
    if (e.target === overlay) closeModal();
  });
  modalCancelBtn.addEventListener("click", closeModal);

  modalOkBtn.addEventListener("click", function () {
    if (!pendingDraft) return;
    var draft = pendingDraft;
    modalOkBtn.disabled = true;

    fetchJSON("api/journal/apply", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        heading: draft.heading,
        insertionMarkdown: draft.insertionMarkdown,
      }),
    })
      .then(function (result) {
        modalOkBtn.disabled = false;
        if (!result.ok) {
          modalReply.textContent = result.data.error || "Failed to update the note.";
          return;
        }
        closeModal();
        if (dailyButton.classList.contains("active")) {
          dailyButton.click();
        }
      })
      .catch(function () {
        modalOkBtn.disabled = false;
        modalReply.textContent = "Failed to update the note.";
      });
  });

  // --- Send ---
  function send() {
    var text = input.value.trim();
    if (!text) return;

    sendBtn.disabled = true;
    status.textContent = "Thinking…";

    fetchJSON("api/journal/draft", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ userText: text }),
    })
      .then(function (result) {
        sendBtn.disabled = false;
        if (!result.ok) {
          status.textContent = result.data.error || "Failed to reach oh-two.";
          return;
        }
        status.textContent = "";
        input.value = "";
        openModal(result.data);
      })
      .catch(function () {
        sendBtn.disabled = false;
        status.textContent = "Failed to reach oh-two.";
      });
  }

  sendBtn.addEventListener("click", send);
  input.addEventListener("keydown", function (e) {
    if (e.key === "Enter") send();
  });

  // --- Visibility: available (NanoClaw reachable) AND Daily is active ---
  var available = false;

  function updateBarVisibility() {
    var dailyActive = dailyButton.classList.contains("active");
    bar.classList.toggle("hidden", !(available && dailyActive));
  }

  function refreshAvailability() {
    fetchJSON("api/journal/status")
      .then(function (result) {
        available = !!(result.ok && result.data.available);
        updateBarVisibility();
      })
      .catch(function () {
        available = false;
        updateBarVisibility();
      });
  }

  new MutationObserver(updateBarVisibility).observe(dailyButton, {
    attributes: true,
    attributeFilter: ["class"],
  });

  refreshAvailability();
  setInterval(refreshAvailability, AVAILABILITY_RECHECK_MS);
})();
