// Foldable headings for rendered note content — see main-randoread.md
// 05.02.03. Shared across every place rendered markdown appears (Daily/
// Rando/Clipped, Watching It Later's Notes panel) since goldmark renders
// headings identically everywhere (see internal/markdown): flat sibling
// elements, not nested under their heading — same shape as
// emoji-picker.js/vim-mode.js (a standalone widget the other frontend files
// call into), not a per-page thing.
//
// Folding a heading hides every element after it up to (not including) the
// next heading of equal-or-higher level — the same "outline fold" semantics
// Obsidian/Notion use, so a folded H2 takes its H3s with it but an H3
// doesn't swallow a sibling H3's own content.
(function () {
  "use strict";

  var HEADING_SELECTOR = "h1, h2, h3, h4, h5, h6";

  function headingLevel(el) {
    return Number(el.tagName.charAt(1));
  }

  function sectionElements(heading) {
    var level = headingLevel(heading);
    var out = [];
    var el = heading.nextElementSibling;
    while (el && !(el.matches(HEADING_SELECTOR) && headingLevel(el) <= level)) {
      out.push(el);
      el = el.nextElementSibling;
    }
    return out;
  }

  function setFolded(heading, folded) {
    heading.classList.toggle("heading-folded", folded);
    var toggle = heading.querySelector(".heading-fold-toggle");
    if (toggle) {
      toggle.textContent = folded ? "▸" : "▾";
      toggle.setAttribute("aria-expanded", String(!folded));
    }
    sectionElements(heading).forEach(function (el) {
      el.classList.toggle("heading-section-collapsed", folded);
    });
  }

  // Idempotent — skips headings that already have a toggle, so calling this
  // more than once on the same content (shouldn't normally happen, but
  // costs nothing to guard) doesn't double up toggles.
  function enhanceFoldableHeadings(container) {
    if (!container) return;
    container.querySelectorAll(HEADING_SELECTOR).forEach(function (heading) {
      if (heading.querySelector(".heading-fold-toggle")) return;

      var toggle = document.createElement("span");
      toggle.className = "heading-fold-toggle";
      toggle.textContent = "▾";
      toggle.setAttribute("role", "button");
      toggle.setAttribute("aria-expanded", "true");
      toggle.setAttribute("aria-label", "Toggle section");
      heading.insertBefore(toggle, heading.firstChild);
    });
  }

  // One delegated listener for the whole page rather than per-container —
  // every container this applies to (#note-content, .watching-notes-body,
  // .watching-notes-preview) gets its innerHTML replaced wholesale on every
  // render anyway, so per-element listeners would just leak.
  document.body.addEventListener("click", function (event) {
    var toggle = event.target.closest(".heading-fold-toggle");
    if (!toggle) return;
    var heading = toggle.closest(HEADING_SELECTOR);
    if (!heading) return;
    setFolded(heading, !heading.classList.contains("heading-folded"));
  });

  window.enhanceFoldableHeadings = enhanceFoldableHeadings;
})();
