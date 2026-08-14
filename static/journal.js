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
// "Security"). Unlike the Daily-only visibility above, an unavailable
// NanoClaw does NOT hide the bar — it stays visible with its input/send
// controls disabled and an explanatory message, so the feature reads as
// "temporarily unavailable" rather than "doesn't exist here." Checked on
// load and rechecked periodically, since reachability can change after
// load (dev laptop sleeping/waking, tailnet dropping) — see
// updateBarVisibility.
(function () {
  "use strict";

  var STORAGE_TOKEN_KEY = "randoread.token";
  var AVAILABILITY_RECHECK_MS = 60000;

  var dailyButton = document.getElementById("daily-button");
  var noteContent = document.getElementById("note-content");
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

  function delay(ms) {
    return new Promise(function (resolve) {
      setTimeout(resolve, ms);
    });
  }

  // One silent retry for a network-layer failure — fetch() itself
  // rejecting (e.g. Chrome's ERR_NETWORK_CHANGED), not a normal HTTP error
  // response, which fetchJSON already resolves as {ok:false, data} rather
  // than rejecting. Specifically absorbs testing from doylestone02 itself:
  // NanoClaw's own container spin-up briefly touches that host's network
  // stack, which can abort an in-flight same-host request even though
  // nothing is actually wrong server-side (main.md §05.05). onRetry, if
  // given, fires once, right before the retry attempt.
  function fetchJSONWithRetry(path, options, onRetry) {
    return fetchJSON(path, options).catch(function () {
      if (onRetry) onRetry();
      return delay(400).then(function () {
        return fetchJSON(path, options);
      });
    });
  }

  // RFC3339 with the browser's own real UTC offset — e.g.
  // "2026-08-14T12:20:00+01:00" during BST. Date.toISOString() always
  // normalizes to UTC ("Z"), which is exactly the bug this replaces: the
  // backend used to timestamp entries with its own server clock (a
  // container, not reliably in the user's timezone), so a BST user saw UTC
  // times in their notes. Only the browser actually knows the user's real
  // local time, so it's computed here and threaded through both the draft
  // and the matching apply request — see journal_draft.go/journal_apply.go's
  // resolveNow doc comments.
  function localIso() {
    var d = new Date();
    function pad(n) {
      return String(n).padStart(2, "0");
    }
    var offsetMin = -d.getTimezoneOffset();
    var sign = offsetMin >= 0 ? "+" : "-";
    var offH = pad(Math.floor(Math.abs(offsetMin) / 60));
    var offM = pad(Math.abs(offsetMin) % 60);
    return (
      d.getFullYear() +
      "-" +
      pad(d.getMonth() + 1) +
      "-" +
      pad(d.getDate()) +
      "T" +
      pad(d.getHours()) +
      ":" +
      pad(d.getMinutes()) +
      ":" +
      pad(d.getSeconds()) +
      sign +
      offH +
      ":" +
      offM
    );
  }

  // --- Floating bar ---
  var bar = document.createElement("div");
  bar.className = "journal-bar hidden";
  var defaultPlaceholder = "Tell oh-two something to note down…";
  var captionPlaceholder = "Add a caption for this photo…";

  bar.innerHTML =
    '<button type="button" class="journal-attach-btn" title="Attach a photo">📎</button>' +
    // No `capture` attribute — that forces straight to the camera on
    // mobile, skipping past the picker. Plain accept="image/*" lets the
    // browser show its normal chooser (camera / gallery / files), same as
    // desktop.
    '<input type="file" class="journal-image-input hidden" accept="image/*">' +
    '<input type="text" class="journal-input" placeholder="' + defaultPlaceholder + '">' +
    '<button type="button" class="journal-send-btn">' +
    '<span class="journal-spinner hidden"></span>' +
    '<span class="journal-send-label">Send to oh-two</span>' +
    "</button>" +
    '<span class="journal-status"></span>' +
    '<span class="journal-unavailable-message hidden">oh-two isn’t reachable — doylestone02 may be off the Tailscale network.</span>' +
    '<div class="journal-image-preview hidden">' +
    '<img class="journal-image-preview-thumb" alt="">' +
    '<button type="button" class="journal-image-preview-remove" aria-label="Remove photo">✕</button>' +
    "</div>";
  document.body.appendChild(bar);

  var input = bar.querySelector(".journal-input");
  var sendBtn = bar.querySelector(".journal-send-btn");
  var sendSpinner = bar.querySelector(".journal-spinner");
  var sendLabel = bar.querySelector(".journal-send-label");
  var status = bar.querySelector(".journal-status");
  var unavailableMessage = bar.querySelector(".journal-unavailable-message");
  var attachBtn = bar.querySelector(".journal-attach-btn");
  var imageInput = bar.querySelector(".journal-image-input");
  var imagePreview = bar.querySelector(".journal-image-preview");
  var imagePreviewThumb = bar.querySelector(".journal-image-preview-thumb");
  var imagePreviewRemoveBtn = bar.querySelector(".journal-image-preview-remove");

  // The photo itself is only ever uploaded at confirm time (see
  // journal_apply.go's doc comment on why) — this just holds the picked
  // File client-side between attach and send.
  var attachedImageFile = null;

  function clearAttachedImage() {
    attachedImageFile = null;
    imageInput.value = "";
    imagePreview.classList.add("hidden");
    if (imagePreviewThumb.src) {
      URL.revokeObjectURL(imagePreviewThumb.src);
      imagePreviewThumb.src = "";
    }
    input.placeholder = defaultPlaceholder;
  }

  attachBtn.addEventListener("click", function () {
    imageInput.click();
  });

  imageInput.addEventListener("change", function () {
    var file = imageInput.files && imageInput.files[0];
    if (!file) return;
    attachedImageFile = file;
    imagePreviewThumb.src = URL.createObjectURL(file);
    imagePreview.classList.remove("hidden");
    // A photo with no caption is exactly what journal_apply.go's mandatory
    // heading/insertionMarkdown check already blocks (send() below never
    // allows empty text through) — the placeholder just makes clear why,
    // rather than requiring a caption silently.
    input.placeholder = captionPlaceholder;
    input.focus();
  });

  imagePreviewRemoveBtn.addEventListener("click", clearAttachedImage);

  // sendBtn.disabled has two independent reasons to be true — mid-request
  // (setSending) and NanoClaw unreachable (updateBarVisibility) — compose
  // them here rather than letting whichever ran last clobber the other
  // (e.g. availability's periodic recheck firing mid-request would
  // otherwise re-enable a button that's still actually sending).
  var sending = false;

  function refreshSendButtonState() {
    sendBtn.disabled = sending || !available;
  }

  // The container spawn NanoClaw does per request genuinely takes several
  // seconds (a real agent turn, not a cheap lookup) — "Thinking…" text
  // alone was too easy to miss, so the button itself shows a spinner for
  // the whole time it's disabled.
  function setSending(isSending) {
    sending = isSending;
    refreshSendButtonState();
    sendSpinner.classList.toggle("hidden", !isSending);
    sendLabel.textContent = isSending ? "Sending…" : "Send to oh-two";
  }

  // --- Confirm modal (same overlay/modal shape as emoji-picker.js) ---
  var overlay = document.createElement("div");
  overlay.className = "journal-modal-overlay";
  overlay.innerHTML =
    '<div class="journal-modal">' +
    '<p class="journal-modal-reply"></p>' +
    '<pre class="journal-modal-insertion"></pre>' +
    '<div class="journal-modal-actions">' +
    '<button type="button" class="journal-modal-btn cancel">Don’t update and close</button>' +
    '<button type="button" class="journal-modal-btn ok">' +
    '<span class="journal-spinner hidden"></span>' +
    '<span class="journal-modal-ok-label">OK</span>' +
    "</button>" +
    "</div>" +
    "</div>";
  document.body.appendChild(overlay);

  var modalReply = overlay.querySelector(".journal-modal-reply");
  var modalInsertion = overlay.querySelector(".journal-modal-insertion");
  var modalCancelBtn = overlay.querySelector(".cancel");
  var modalOkBtn = overlay.querySelector(".ok");
  var modalOkSpinner = modalOkBtn.querySelector(".journal-spinner");
  var modalOkLabel = modalOkBtn.querySelector(".journal-modal-ok-label");

  function setApplying(applying) {
    modalOkBtn.disabled = applying;
    modalOkSpinner.classList.toggle("hidden", !applying);
    modalOkLabel.textContent = applying ? "Saving…" : "OK";
  }

  var pendingDraft = null; // {heading, insertionMarkdown, reply, nowIso} awaiting confirm

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

  function stripMarkdownHeading(md) {
    return (md || "").replace(/^#+\s*/, "").trim();
  }

  function headingLevelOf(el) {
    return Number(el.tagName.charAt(1));
  }

  // enhanceFoldableHeadings (foldable-headings.js) prepends a ▾/▸ toggle
  // glyph as the heading's first child — strip it before comparing text.
  function renderedHeadingText(heading) {
    return heading.textContent.replace(/^[▾▸]\s*/, "").trim();
  }

  // After a note reload that just added a line, collapse every top-level
  // (H2) section except the one the new line actually landed in, so the
  // addition is immediately visible without scrolling the whole note —
  // one-shot, only for the reload this triggers, not general Daily
  // browsing. headingMarkdown is the exact heading NanoClaw returned (e.g.
  // "## 📌 etc." or "### Vent") — for a ### MOTIVES subheading, this walks
  // back to its enclosing "## 🗨 Log" and keeps that expanded instead,
  // since folding the parent would hide the subheading entirely.
  function revealSectionAfterReload(headingMarkdown) {
    if (!noteContent || !window.setHeadingFolded) return;
    var targetText = stripMarkdownHeading(headingMarkdown);

    // app.js's loadDaily clears #note-content to empty synchronously
    // *before* the async fetch for the real content resolves — a
    // childList observer fires on that clear too, as its own, earlier
    // mutation record. Disconnecting on the first callback (rather than
    // waiting for one that actually contains headings) meant this fired
    // on the empty interim state and never saw the real content — the
    // fold/scroll effect silently never ran, even though the reload and
    // the write it followed both succeeded normally.
    var giveUp = setTimeout(function () {
      observer.disconnect();
    }, 10000);

    var observer = new MutationObserver(function () {
      var headings = Array.prototype.slice.call(
        noteContent.querySelectorAll("h1, h2, h3, h4, h5, h6"),
      );
      if (headings.length === 0) return; // still the cleared interim state — keep waiting

      var targetIdx = -1;
      for (var i = 0; i < headings.length; i++) {
        if (renderedHeadingText(headings[i]) === targetText) {
          targetIdx = i;
          break;
        }
      }
      if (targetIdx === -1) {
        // Real content arrived but the heading wasn't found (shouldn't
        // normally happen) — give up rather than keep watching forever.
        clearTimeout(giveUp);
        observer.disconnect();
        return;
      }

      clearTimeout(giveUp);
      observer.disconnect();

      var keepExpanded = headings[targetIdx];
      for (var j = targetIdx; j >= 0; j--) {
        if (headingLevelOf(headings[j]) <= 2) {
          keepExpanded = headings[j];
          break;
        }
      }

      headings.forEach(function (h) {
        if (headingLevelOf(h) <= 2) {
          window.setHeadingFolded(h, h !== keepExpanded);
        }
      });

      keepExpanded.scrollIntoView({ block: "start", behavior: "smooth" });
    });
    observer.observe(noteContent, { childList: true });
  }

  modalOkBtn.addEventListener("click", function () {
    if (!pendingDraft) return;
    var draft = pendingDraft;
    setApplying(true);

    // multipart/form-data, not JSON — journal_apply.go accepts an optional
    // "image" file part alongside the text fields (the photo is uploaded
    // here, at confirm time, not at draft time — see its doc comment on
    // why). No Content-Type header set: the browser fills in the correct
    // multipart boundary itself for a FormData body.
    var formData = new FormData();
    formData.append("heading", draft.heading);
    formData.append("insertionMarkdown", draft.insertionMarkdown);
    // Same nowIso the matching draft request sent — see journal_apply.go's
    // resolveNow doc comment on why apply must target the same day draft
    // did, not re-derive "now" again.
    formData.append("nowIso", draft.nowIso);
    if (draft.imageFile) {
      formData.append("image", draft.imageFile);
    }

    fetchJSONWithRetry("api/journal/apply", {
      method: "POST",
      body: formData,
    })
      .then(function (result) {
        setApplying(false);
        if (!result.ok) {
          modalReply.textContent = result.data.error || "Failed to update the note.";
          return;
        }
        closeModal();
        if (dailyButton.classList.contains("active")) {
          revealSectionAfterReload(draft.heading);
          dailyButton.click();
        }
      })
      .catch(function () {
        setApplying(false);
        modalReply.textContent = "Failed to update the note.";
      });
  });

  // --- Send ---
  function send() {
    var text = input.value.trim();
    // A caption is mandatory when a photo is attached — this same empty
    // check already covers that, no separate validation needed: an
    // attached image with no text just doesn't send, same as no image.
    if (!text) return;

    var nowIso = localIso();
    var imageFile = attachedImageFile; // captured now — see openModal's comment
    setSending(true);
    status.textContent = "";

    fetchJSONWithRetry(
      "api/journal/draft",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ userText: text, nowIso: nowIso }),
      },
      function () {
        status.textContent = "Connection hiccup, retrying…";
      },
    )
      .then(function (result) {
        setSending(false);
        if (!result.ok) {
          status.textContent = result.data.error || "Failed to reach oh-two.";
          return;
        }
        status.textContent = "";
        input.value = "";
        // The image itself isn't uploaded until apply — carrying the File
        // on the draft result here just gets it to that step; clearing the
        // attach UI now (rather than after apply) lets the next entry
        // start with a clean slate immediately, same as input.value above.
        clearAttachedImage();
        openModal(Object.assign({}, result.data, { nowIso: nowIso, imageFile: imageFile }));
      })
      .catch(function () {
        setSending(false);
        status.textContent = "Failed to reach oh-two.";
      });
  }

  sendBtn.addEventListener("click", send);
  input.addEventListener("keydown", function (e) {
    if (e.key === "Enter") send();
  });

  // --- Visibility: shown whenever Daily is active, regardless of
  // availability — NanoClaw being unreachable disables the controls and
  // explains why (below) rather than hiding the bar outright, so the
  // feature reads as "temporarily unavailable," not "doesn't exist here."
  var available = false;

  function updateBarVisibility() {
    var dailyActive = dailyButton.classList.contains("active");
    bar.classList.toggle("hidden", !dailyActive);
    input.disabled = !available;
    attachBtn.disabled = !available;
    refreshSendButtonState();
    unavailableMessage.classList.toggle("hidden", available);
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
