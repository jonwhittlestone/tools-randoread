(function () {
  "use strict";

  const STORAGE_TOKEN_KEY = "randoread.token";
  const STORAGE_EXPIRES_KEY = "randoread.expiresAt";

  const loginScreen = document.getElementById("login-screen");
  const app = document.getElementById("app");

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
    } else {
      showLogin();
    }
  }

  init();
})();
