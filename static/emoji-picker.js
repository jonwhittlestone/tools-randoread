// EmojiPicker — a small search/grid modal for tagging the currently staged
// "Watching It Later" video with an emoji. Copied as-is from
// tools-watchitlater's static/emoji-picker.js (itself ported from
// 25-tools-browsernotes' web/EmojiPicker.ts) — same overlay + search + grid
// + OK/Cancel UX, themed with this project's own CSS custom properties
// (light/dark aware) instead of a fixed dark palette.
(function () {
  "use strict";

  var EMOJI_LIST = [
    { emoji: "😀", keywords: ["grin", "happy", "smile"] },
    { emoji: "😊", keywords: ["blush", "happy", "smile"] },
    { emoji: "😂", keywords: ["laugh", "cry", "funny"] },
    { emoji: "🥹", keywords: ["hold", "tears", "touched"] },
    { emoji: "😍", keywords: ["love", "heart", "eyes"] },
    { emoji: "😎", keywords: ["cool", "sunglasses"] },
    { emoji: "🤔", keywords: ["think", "hmm", "wonder"] },
    { emoji: "😴", keywords: ["sleep", "zzz", "tired"] },
    { emoji: "😤", keywords: ["angry", "huff", "frustrated"] },
    { emoji: "😢", keywords: ["cry", "sad", "tear"] },
    { emoji: "🤯", keywords: ["mind", "blown", "explode"] },
    { emoji: "🥳", keywords: ["party", "celebrate", "birthday"] },
    { emoji: "😇", keywords: ["angel", "innocent", "halo"] },
    { emoji: "🤗", keywords: ["hug", "warm"] },
    { emoji: "🫡", keywords: ["salute", "respect"] },
    { emoji: "👍", keywords: ["thumb", "up", "yes", "good"] },
    { emoji: "👎", keywords: ["thumb", "down", "no", "bad"] },
    { emoji: "👏", keywords: ["clap", "applause", "bravo"] },
    { emoji: "🙌", keywords: ["raise", "hands", "hooray", "celebrate"] },
    { emoji: "🤝", keywords: ["handshake", "deal", "agree"] },
    { emoji: "✌️", keywords: ["peace", "victory"] },
    { emoji: "💪", keywords: ["strong", "muscle", "flex"] },
    { emoji: "🫶", keywords: ["heart", "hands", "love"] },
    { emoji: "❤️", keywords: ["heart", "love", "red"] },
    { emoji: "💛", keywords: ["heart", "yellow"] },
    { emoji: "💚", keywords: ["heart", "green"] },
    { emoji: "💙", keywords: ["heart", "blue"] },
    { emoji: "💜", keywords: ["heart", "purple"] },
    { emoji: "🖤", keywords: ["heart", "black"] },
    { emoji: "⭐", keywords: ["star", "gold", "favorite"] },
    { emoji: "🌟", keywords: ["star", "glow", "sparkle"] },
    { emoji: "✨", keywords: ["sparkle", "magic", "clean"] },
    { emoji: "🔥", keywords: ["fire", "hot", "lit"] },
    { emoji: "💯", keywords: ["hundred", "perfect", "score"] },
    { emoji: "⚡", keywords: ["lightning", "electric", "fast", "energy"] },
    { emoji: "💡", keywords: ["idea", "bulb", "light"] },
    { emoji: "🎯", keywords: ["target", "goal", "bullseye", "aim"] },
    { emoji: "🌈", keywords: ["rainbow", "colors"] },
    { emoji: "☀️", keywords: ["sun", "sunny", "bright"] },
    { emoji: "🌙", keywords: ["moon", "night", "crescent"] },
    { emoji: "🌧️", keywords: ["rain", "cloud"] },
    { emoji: "❄️", keywords: ["snow", "cold", "winter", "ice"] },
    { emoji: "🌸", keywords: ["cherry", "blossom", "flower", "spring"] },
    { emoji: "🌻", keywords: ["sunflower", "flower"] },
    { emoji: "🍀", keywords: ["clover", "luck", "lucky", "four"] },
    { emoji: "🌲", keywords: ["tree", "evergreen", "pine"] },
    { emoji: "🌊", keywords: ["wave", "ocean", "sea", "water"] },
    { emoji: "🐶", keywords: ["dog", "puppy", "pet"] },
    { emoji: "🐱", keywords: ["cat", "kitty", "pet"] },
    { emoji: "🐻", keywords: ["bear", "teddy"] },
    { emoji: "🦊", keywords: ["fox"] },
    { emoji: "🐍", keywords: ["snake", "python"] },
    { emoji: "🦅", keywords: ["eagle", "bird"] },
    { emoji: "🐝", keywords: ["bee", "honey", "busy"] },
    { emoji: "🦋", keywords: ["butterfly", "beautiful"] },
    { emoji: "🐢", keywords: ["turtle", "slow"] },
    { emoji: "🐬", keywords: ["dolphin", "ocean"] },
    { emoji: "🍎", keywords: ["apple", "red", "fruit"] },
    { emoji: "🍕", keywords: ["pizza", "food"] },
    { emoji: "🍔", keywords: ["burger", "hamburger", "food"] },
    { emoji: "🌮", keywords: ["taco", "food", "mexican"] },
    { emoji: "🍜", keywords: ["noodle", "ramen", "soup"] },
    { emoji: "🍰", keywords: ["cake", "dessert", "sweet"] },
    { emoji: "🍩", keywords: ["donut", "doughnut", "sweet"] },
    { emoji: "☕", keywords: ["coffee", "tea", "hot", "drink"] },
    { emoji: "🍺", keywords: ["beer", "drink", "cheers"] },
    { emoji: "🧃", keywords: ["juice", "box", "drink"] },
    { emoji: "⚽", keywords: ["soccer", "football", "sport"] },
    { emoji: "🏀", keywords: ["basketball", "sport"] },
    { emoji: "🎾", keywords: ["tennis", "sport"] },
    { emoji: "🏃", keywords: ["run", "jog", "exercise"] },
    { emoji: "🚴", keywords: ["bike", "cycle", "bicycle"] },
    { emoji: "🏊", keywords: ["swim", "pool", "water"] },
    { emoji: "🎮", keywords: ["game", "controller", "play", "video"] },
    { emoji: "🎬", keywords: ["movie", "film", "cinema", "clapper"] },
    { emoji: "🎵", keywords: ["music", "note", "song"] },
    { emoji: "🎸", keywords: ["guitar", "music", "rock"] },
    { emoji: "🎨", keywords: ["art", "paint", "palette", "creative"] },
    { emoji: "📱", keywords: ["phone", "mobile", "cell"] },
    { emoji: "💻", keywords: ["laptop", "computer"] },
    { emoji: "📝", keywords: ["note", "memo", "write", "pencil"] },
    { emoji: "📚", keywords: ["books", "read", "study", "library"] },
    { emoji: "🔧", keywords: ["wrench", "tool", "fix", "repair"] },
    { emoji: "🏠", keywords: ["house", "home"] },
    { emoji: "🚗", keywords: ["car", "drive", "auto"] },
    { emoji: "✈️", keywords: ["plane", "airplane", "travel", "flight"] },
    { emoji: "🚀", keywords: ["rocket", "launch", "space", "fast"] },
    { emoji: "✅", keywords: ["check", "done", "complete", "yes"] },
    { emoji: "❌", keywords: ["cross", "no", "wrong", "delete"] },
    { emoji: "⚠️", keywords: ["warning", "caution", "alert"] },
    { emoji: "❓", keywords: ["question", "what", "help"] },
    { emoji: "🎉", keywords: ["party", "celebrate", "tada", "confetti"] },
    { emoji: "🏆", keywords: ["trophy", "winner", "champion", "award"] },
    { emoji: "🧘", keywords: ["yoga", "meditate", "zen", "calm"] },
    { emoji: "🧠", keywords: ["brain", "think", "smart", "mind"] },
    { emoji: "👀", keywords: ["eyes", "look", "see", "watch"] },
    { emoji: "🧑‍💻", keywords: ["developer", "code", "programmer", "tech"] },
    { emoji: "🎧", keywords: ["headphones", "music", "listen"] },
    { emoji: "📺", keywords: ["tv", "television", "watch"] },
    { emoji: "⚽", keywords: ["football", "sport"] },
    { emoji: "🏋️", keywords: ["weight", "lift", "gym", "exercise"] },
  ];

  var instance = null;

  function EmojiPicker() {
    var self = this;

    this.overlay = document.createElement("div");
    this.overlay.className = "emoji-picker-overlay";

    this.modal = document.createElement("div");
    this.modal.className = "emoji-picker-modal";

    this.searchInput = document.createElement("input");
    this.searchInput.type = "text";
    this.searchInput.className = "emoji-picker-search";
    this.searchInput.placeholder = "Search or paste emoji...";
    this.searchInput.addEventListener("input", function () {
      self.filterEmojis();
    });
    this.searchInput.addEventListener("keydown", function (e) {
      if (e.key === "Enter") self.close(self.getResult());
      if (e.key === "Escape") self.close(null);
    });

    this.grid = document.createElement("div");
    this.grid.className = "emoji-picker-grid";

    var actions = document.createElement("div");
    actions.className = "emoji-picker-actions";

    var cancelBtn = document.createElement("button");
    cancelBtn.type = "button";
    cancelBtn.className = "emoji-picker-btn cancel";
    cancelBtn.textContent = "Cancel";
    cancelBtn.addEventListener("click", function () {
      self.close(null);
    });

    var okBtn = document.createElement("button");
    okBtn.type = "button";
    okBtn.className = "emoji-picker-btn ok";
    okBtn.textContent = "OK";
    okBtn.addEventListener("click", function () {
      self.close(self.getResult());
    });

    actions.appendChild(cancelBtn);
    actions.appendChild(okBtn);

    this.modal.appendChild(this.searchInput);
    this.modal.appendChild(this.grid);
    this.modal.appendChild(actions);
    this.overlay.appendChild(this.modal);

    this.overlay.addEventListener("click", function (e) {
      if (e.target === self.overlay) self.close(null);
    });

    document.body.appendChild(this.overlay);

    this.resolve = null;
    this.selectedEmoji = "";
  }

  EmojiPicker.prototype.open = function (currentEmoji) {
    var self = this;
    this.selectedEmoji = currentEmoji || "";
    this.searchInput.value = this.selectedEmoji;
    this.renderGrid(EMOJI_LIST);
    this.overlay.classList.add("visible");
    this.searchInput.focus();

    return new Promise(function (resolve) {
      self.resolve = resolve;
    });
  };

  EmojiPicker.prototype.close = function (result) {
    this.overlay.classList.remove("visible");
    if (this.resolve) {
      this.resolve(result);
      this.resolve = null;
    }
  };

  EmojiPicker.prototype.getResult = function () {
    return this.searchInput.value.trim();
  };

  EmojiPicker.prototype.filterEmojis = function () {
    var query = this.searchInput.value.toLowerCase().trim();
    if (!query) {
      this.renderGrid(EMOJI_LIST);
      return;
    }

    var filtered = EMOJI_LIST.filter(function (e) {
      return (
        e.emoji.indexOf(query) !== -1 ||
        e.keywords.some(function (k) {
          return k.indexOf(query) !== -1;
        })
      );
    });
    this.renderGrid(filtered);
  };

  EmojiPicker.prototype.renderGrid = function (emojis) {
    var self = this;
    this.grid.innerHTML = "";
    emojis.forEach(function (entry) {
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "emoji-picker-emoji";
      btn.textContent = entry.emoji;
      btn.title = entry.keywords.join(", ");
      btn.addEventListener("click", function () {
        self.selectedEmoji = entry.emoji;
        self.searchInput.value = entry.emoji;
      });
      self.grid.appendChild(btn);
    });
  };

  function getInstance() {
    if (!instance) instance = new EmojiPicker();
    return instance;
  }

  window.WatchItLaterEmojiPicker = {
    // show returns a Promise resolving to: the chosen emoji string, "" if
    // cleared, or null if cancelled.
    show: function (currentEmoji) {
      return getInstance().open(currentEmoji);
    },
  };
})();
