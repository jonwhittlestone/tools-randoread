// Per-video notes pane for "Watching It Later" — see main-randoread.md
// section 05.02. Self-contained (own authedFetch, own delegated listeners on
// #note-content), same split-out-of-app.js shape as emoji-picker.js, except
// this one talks to the network itself rather than being a pure UI widget —
// it owns the GET/save/search/related-note round trips for the notes panel.
//
// Autosaves on input (debounced ~1.5s), never on an explicit Save click —
// see the feature plan's "why" for this. Nothing is written to Dropbox until
// the first autosave actually fires.
(function () {
  "use strict";

  var STORAGE_TOKEN_KEY = "randoread.token";
  var SAVE_DEBOUNCE_MS = 1500;
  var SEARCH_DEBOUNCE_MS = 250;

  var noteContent = document.getElementById("note-content");
  if (!noteContent) return;

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

  var SKELETON_HTML =
    // fix 3 (05.02.02): note controls right-justified on the same line as
    // the "main | related note" breadcrumb, not stacked below the body.
    '<div class="watching-notes-header-row">' +
    '<div class="watching-notes-refs"></div>' +
    '<div class="watching-notes-actions hidden">' +
    '<button type="button" class="watching-notes-edit-btn">✏️ Edit</button>' +
    '<button type="button" class="watching-notes-done-btn hidden">Done</button>' +
    '<button type="button" class="watching-notes-link-btn">🔗 Linking to existing note</button>' +
    "</div>" +
    "</div>" +
    '<div class="watching-notes-preview hidden"></div>' +
    '<div class="watching-notes-body"></div>' +
    '<div class="watching-notes-editor hidden">' +
    '<textarea class="watching-notes-textarea" spellcheck="false"></textarea>' +
    '<div class="watching-notes-editor-toolbar">' +
    '<button type="button" class="watching-notes-vim-toggle">Vim: off</button>' +
    '<span class="watching-notes-vim-indicator"></span>' +
    '<span class="watching-notes-save-status"></span>' +
    "</div>" +
    "</div>" +
    '<div class="watching-notes-picker hidden">' +
    '<input type="text" class="watching-notes-picker-input" placeholder="Search vault notes…">' +
    '<ul class="watching-notes-picker-results"></ul>' +
    "</div>";

  function panelEls(panel) {
    return {
      refs: panel.querySelector(".watching-notes-refs"),
      preview: panel.querySelector(".watching-notes-preview"),
      body: panel.querySelector(".watching-notes-body"),
      actions: panel.querySelector(".watching-notes-actions"),
      editorWrap: panel.querySelector(".watching-notes-editor"),
      textarea: panel.querySelector(".watching-notes-textarea"),
      vimIndicator: panel.querySelector(".watching-notes-vim-indicator"),
      vimToggle: panel.querySelector(".watching-notes-vim-toggle"),
      saveStatus: panel.querySelector(".watching-notes-save-status"),
      editBtn: panel.querySelector(".watching-notes-edit-btn"),
      doneBtn: panel.querySelector(".watching-notes-done-btn"),
      linkBtn: panel.querySelector(".watching-notes-link-btn"),
      picker: panel.querySelector(".watching-notes-picker"),
      pickerInput: panel.querySelector(".watching-notes-picker-input"),
      pickerResults: panel.querySelector(".watching-notes-picker-results"),
    };
  }

  // --- Per-panel state (a fresh .watching-notes-panel node is created every
  // time app.js re-renders Watching It Later — e.g. a new video staged — so
  // WeakMaps keyed on that node need no manual cleanup; the old node and its
  // entries are simply unreachable once replaced). ---------------------
  var saveTimers = new WeakMap();
  var pendingContent = new WeakMap();
  var vimInstances = new WeakMap();
  // fix 2 (05.02.02): the note body's own "## vault references" links (real
  // markdown links rendered server-side, pointing at raw vault paths — not
  // servable URLs) need to be routed through the same inline preview as the
  // breadcrumb, not left to navigate the browser away. Tracked per panel so
  // the delegated click handler can tell "known reference link" apart from
  // an ordinary external link (e.g. the note's YouTube/source URL).
  var referencePaths = new WeakMap();

  function renderReferences(panel, references) {
    var els = panelEls(panel);
    els.refs.innerHTML = "";
    referencePaths.set(
      panel,
      (references || []).map(function (ref) {
        return ref.path;
      })
    );

    // fix 1: "main" is clickable (not just a static label) — it re-fetches
    // and shows this video's own note, same as any other breadcrumb entry.
    var main = document.createElement("a");
    main.href = "#";
    main.className = "watching-notes-ref-main";
    main.textContent = "main";
    els.refs.appendChild(main);

    (references || []).forEach(function (ref) {
      var sep = document.createElement("span");
      sep.className = "watching-notes-ref-sep";
      sep.textContent = " | ";
      els.refs.appendChild(sep);

      var link = document.createElement("a");
      link.href = "#";
      link.className = "watching-notes-ref-link";
      link.dataset.path = ref.path;
      link.textContent = ref.title;
      els.refs.appendChild(link);
    });
  }

  function fillPanel(panel, data) {
    var els = panelEls(panel);
    panel.dataset.path = data.path || "";
    panel.dataset.raw = data.raw || "";
    els.body.innerHTML = data.html || "";
    renderReferences(panel, data.references);
    // Edit/Link only make sense once we actually know the note's current
    // content — showing them earlier let a fast click into Edit start from
    // an empty textarea and autosave over the real content before the
    // initial GET had even resolved (found live; see PR history).
    els.actions.classList.remove("hidden");
  }

  // Fetches this video's own note fresh from Dropbox and fills panel with
  // it. Shared by the initial load and by re-clicking "main" (fix 4: a
  // header link click always re-fetches, it's never served stale/cached).
  function fetchAndFillMain(panel) {
    var els = panelEls(panel);
    return fetchJSON("api/watching/note")
      .then(function (r) {
        if (!r.ok) {
          els.body.textContent = r.data.error || "Failed to load notes.";
          return;
        }
        fillPanel(panel, r.data);
      })
      .catch(function () {
        els.body.textContent = "Failed to load notes.";
      });
  }

  function loadPanel(panel) {
    panel.innerHTML = SKELETON_HTML;
    var els = panelEls(panel);
    els.body.textContent = "Loading…";
    wireEditor(panel);
    wirePicker(panel);
    fetchAndFillMain(panel);
  }

  // fix 4: re-clicking "main" always re-fetches fresh content from Dropbox
  // (rather than just re-showing whatever's already in memory) and drops
  // back out of any open related-note preview.
  function refreshMainNote(panel) {
    var els = panelEls(panel);
    els.preview.classList.add("hidden");
    els.body.textContent = "Loading…";
    fetchAndFillMain(panel);
  }

  function wireEditor(panel) {
    var els = panelEls(panel);
    els.textarea.addEventListener("input", function () {
      scheduleSave(panel);
    });
    els.textarea.addEventListener("blur", function () {
      flushSave(panel);
    });
  }

  function wirePicker(panel) {
    var els = panelEls(panel);
    var debounce = null;
    els.pickerInput.addEventListener("input", function () {
      clearTimeout(debounce);
      debounce = setTimeout(function () {
        runSearch(panel);
      }, SEARCH_DEBOUNCE_MS);
    });
  }

  function setSaveStatus(panel, text) {
    panelEls(panel).saveStatus.textContent = text;
  }

  function nowLabel() {
    var d = new Date();
    function pad(n) {
      return n < 10 ? "0" + n : String(n);
    }
    return pad(d.getHours()) + ":" + pad(d.getMinutes());
  }

  function scheduleSave(panel) {
    var els = panelEls(panel);
    pendingContent.set(panel, els.textarea.value);
    setSaveStatus(panel, "");
    clearTimeout(saveTimers.get(panel));
    saveTimers.set(
      panel,
      setTimeout(function () {
        doSave(panel);
      }, SAVE_DEBOUNCE_MS)
    );
  }

  // Called on blur/collapse/Done — commits a pending debounced edit
  // immediately rather than waiting out the timer, so nothing typed is lost
  // by leaving the editor. Resolves once any in-flight save settles.
  function flushSave(panel) {
    clearTimeout(saveTimers.get(panel));
    if (pendingContent.get(panel) != null) {
      return doSave(panel);
    }
    return Promise.resolve();
  }

  function doSave(panel) {
    var els = panelEls(panel);
    var content = els.textarea.value;
    pendingContent.set(panel, null);
    setSaveStatus(panel, "Saving…");

    return fetchJSON("api/watching/note", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content: content }),
    }).then(function (r) {
      if (!r.ok) {
        setSaveStatus(panel, "Failed to save");
        return;
      }
      fillPanel(panel, r.data);
      setSaveStatus(panel, "Saved ✓ " + nowLabel());
    }).catch(function () {
      setSaveStatus(panel, "Failed to save");
    });
  }

  function vimActive(panel) {
    var vim = vimInstances.get(panel);
    return !!(vim && vim.enabled);
  }

  function toggleVim(panel) {
    var els = panelEls(panel);
    var vim = vimInstances.get(panel);
    if (!vim) {
      vim = new window.VimMode(els.textarea, els.vimIndicator);
      vimInstances.set(panel, vim);
    }
    if (vim.enabled) {
      vim.disable();
      els.vimToggle.textContent = "Vim: off";
    } else {
      vim.enable();
      els.vimToggle.textContent = "Vim: on";
    }
  }

  function enterEditMode(panel) {
    var els = panelEls(panel);
    els.textarea.value = panel.dataset.raw || "";
    els.editorWrap.classList.remove("hidden");
    els.body.classList.add("hidden");
    els.editBtn.classList.add("hidden");
    els.doneBtn.classList.remove("hidden");
    els.textarea.focus();
  }

  function exitEditMode(panel) {
    flushSave(panel).then(function () {
      var els = panelEls(panel);
      if (vimActive(panel)) toggleVim(panel);
      els.editorWrap.classList.add("hidden");
      els.body.classList.remove("hidden");
      els.editBtn.classList.remove("hidden");
      els.doneBtn.classList.add("hidden");
    });
  }

  function runSearch(panel) {
    var els = panelEls(panel);
    var q = els.pickerInput.value.trim();
    els.pickerResults.innerHTML = "";
    if (!q) return;

    fetchJSON("api/watching/note/search?q=" + encodeURIComponent(q))
      .then(function (r) {
        if (!r.ok) return;
        els.pickerResults.innerHTML = "";
        (r.data.results || []).forEach(function (result) {
          var li = document.createElement("li");
          li.className = "watching-notes-picker-result";
          li.dataset.path = result.path;
          li.textContent = result.title;
          els.pickerResults.appendChild(li);
        });
      })
      .catch(function () {});
  }

  function addRelated(panel, path) {
    fetchJSON("api/watching/note/related", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: path }),
    })
      .then(function (r) {
        if (!r.ok) return;
        fillPanel(panel, r.data);
        var els = panelEls(panel);
        els.picker.classList.add("hidden");
      })
      .catch(function () {});
  }

  function toggleRelatedPreview(panel, path) {
    var els = panelEls(panel);
    if (els.preview.dataset.path === path && !els.preview.classList.contains("hidden")) {
      els.preview.classList.add("hidden");
      return;
    }

    els.preview.classList.remove("hidden");
    els.preview.dataset.path = path;
    els.preview.textContent = "Loading…";

    fetchJSON("api/watching/note/related-preview?path=" + encodeURIComponent(path))
      .then(function (r) {
        if (!r.ok) {
          els.preview.textContent = r.data.error || "Failed to load note.";
          return;
        }
        els.preview.innerHTML = r.data.html;
      })
      .catch(function () {
        els.preview.textContent = "Failed to load note.";
      });
  }

  noteContent.addEventListener("click", function (event) {
    var toggle = event.target.closest(".watching-notes-toggle");
    if (toggle) {
      var wrap = toggle.closest(".watching-notes");
      var panel = wrap.querySelector(".watching-notes-panel");
      var collapsedNow = panel.classList.toggle("hidden");
      toggle.textContent = (collapsedNow ? "▸" : "▾") + " Notes";
      toggle.setAttribute("aria-expanded", String(!collapsedNow));
      if (collapsedNow) {
        flushSave(panel);
      } else if (panel.dataset.loaded !== "true") {
        panel.dataset.loaded = "true";
        loadPanel(panel);
      }
      return;
    }

    var editBtn = event.target.closest(".watching-notes-edit-btn");
    if (editBtn) {
      enterEditMode(editBtn.closest(".watching-notes-panel"));
      return;
    }

    var doneBtn = event.target.closest(".watching-notes-done-btn");
    if (doneBtn) {
      exitEditMode(doneBtn.closest(".watching-notes-panel"));
      return;
    }

    var vimToggle = event.target.closest(".watching-notes-vim-toggle");
    if (vimToggle) {
      toggleVim(vimToggle.closest(".watching-notes-panel"));
      return;
    }

    var linkBtn = event.target.closest(".watching-notes-link-btn");
    if (linkBtn) {
      var panelForLink = linkBtn.closest(".watching-notes-panel");
      var els = panelEls(panelForLink);
      var nowHidden = els.picker.classList.toggle("hidden");
      if (!nowHidden) {
        els.pickerInput.value = "";
        els.pickerResults.innerHTML = "";
        els.pickerInput.focus();
      }
      return;
    }

    var mainLink = event.target.closest(".watching-notes-ref-main");
    if (mainLink) {
      event.preventDefault();
      refreshMainNote(mainLink.closest(".watching-notes-panel"));
      return;
    }

    var refLink = event.target.closest(".watching-notes-ref-link");
    if (refLink) {
      event.preventDefault();
      toggleRelatedPreview(refLink.closest(".watching-notes-panel"), refLink.dataset.path);
      return;
    }

    // fix 2 (05.02.02): a "## vault references" link *inside the note body*
    // (real markdown rendered server-side, pointing at a raw vault path —
    // not a servable URL) — route it through the same inline preview the
    // breadcrumb links use instead of letting it navigate nowhere useful.
    // Anything else in the body (the note's own YouTube/source URL) isn't a
    // known reference path, so it falls through and opens normally in a new
    // tab (the renderer already adds target="_blank" — see fix 5).
    var bodyLink = event.target.closest(".watching-notes-body a");
    if (bodyLink) {
      var panelForBodyLink = bodyLink.closest(".watching-notes-panel");
      var knownPaths = referencePaths.get(panelForBodyLink) || [];
      var href = bodyLink.getAttribute("href");
      if (knownPaths.indexOf(href) !== -1) {
        event.preventDefault();
        toggleRelatedPreview(panelForBodyLink, href);
        return;
      }
    }

    var resultItem = event.target.closest(".watching-notes-picker-result");
    if (resultItem) {
      addRelated(resultItem.closest(".watching-notes-panel"), resultItem.dataset.path);
      return;
    }
  });

  // Best-effort flush of an unsaved, still-debounced edit on tab close/nav
  // away — sendBeacon can't carry a custom header, so the token travels as a
  // query param instead, same as the full-page Dropbox OAuth redirect in
  // app.js's dropboxConnectBtn handler (RequireToken accepts either).
  window.addEventListener("beforeunload", function () {
    var panel = noteContent.querySelector(".watching-notes-panel");
    if (!panel) return;
    var content = pendingContent.get(panel);
    if (content == null) return;
    clearTimeout(saveTimers.get(panel));

    var token = localStorage.getItem(STORAGE_TOKEN_KEY);
    var url = "api/watching/note?token=" + encodeURIComponent(token);
    var blob = new Blob([JSON.stringify({ content: content })], { type: "application/json" });
    if (navigator.sendBeacon) navigator.sendBeacon(url, blob);
  });
})();
