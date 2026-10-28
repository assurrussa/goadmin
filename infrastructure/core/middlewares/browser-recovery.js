"use strict";
(() => {
  const status = document.getElementById("auth-recovery-status");
  const retry = document.getElementById("auth-recovery-retry");
  const next = document.getElementById("auth-recovery-continue");
  if (!status || !retry || !next) return;
  const title = document.getElementById("auth-recovery-title");
  const signIn = document.getElementById("auth-recovery-sign-in");
  const guardKey = "goadmin:recovery:auto-return:v1";
  const guardWindow = 60000;
  function recentlyReturned() {
    try {
      const raw = sessionStorage.getItem(guardKey);
      if (raw === null) return false;
      const last = Number(raw);
      return !Number.isFinite(last) || Date.now() - last < guardWindow;
    } catch { return true; }
  }
  function reserveReturn() {
    if (recentlyReturned()) return false;
    try {
      const stamp = String(Date.now());
      sessionStorage.setItem(guardKey, stamp);
      return sessionStorage.getItem(guardKey) === stamp;
    } catch { return false; }
  }
  function heading(text) {
    if (title) title.textContent = text;
    document.title = text;
  }
  let pending = false;
  let leaving = false;
  let activeController;
  let navigationTimer;
  function abandon() {
    leaving = true;
    activeController?.abort();
    if (navigationTimer !== undefined) clearTimeout(navigationTimer);
  }
  if (signIn) signIn.addEventListener("click", event => {
    if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || event.button > 0) return;
    abandon();
  });
  window.addEventListener("pagehide", abandon);
  async function recover() {
    if (pending || leaving) return;
    if (navigationTimer !== undefined) clearTimeout(navigationTimer);
    navigationTimer = undefined;
    pending = true;
    retry.hidden = true;
    next.hidden = true;
    heading("Проверяем вход");
    status.textContent = "Проверяем вашу сессию. Это займёт несколько секунд.";
    const controller = new AbortController();
    activeController = controller;
    const timeout = setTimeout(() => controller.abort(), 30000);
    try {
      // Same fixed contract as the in-page coordinator. No rejected request,
      // body, callback, proof or response-selected destination is retained.
      for (let step = 0; step < 2; step++) {
        const response = await fetch("/auth/session/recover", {
          method: "GET", credentials: "same-origin", mode: "same-origin",
          cache: "no-store", redirect: "error", signal: controller.signal,
          headers: {Accept: "application/json", "X-Goadmin-Auth-Recovery": "1"}
        });
        if (controller.signal.aborted || response.redirected ||
            response.headers.get("x-goadmin-auth-recovery") !== "1" ||
            response.headers.get("content-type")?.split(";")[0].trim().toLowerCase() !== "application/json") break;
        const body = await response.json();
        if (controller.signal.aborted || !body || typeof body !== "object" || Array.isArray(body) || body.status !== response.status) break;
        if (response.status === 200 && body.code === "authentication_recovered" && body.csrfReady === true) {
          heading("Вход подтверждён");
          if (signIn) signIn.hidden = true;
          next.hidden = false;
          status.textContent = "Можно вернуться на страницу. Если переход не начался, нажмите «Продолжить».";
          // Persist before the safe GET navigation. A second recovery document
          // in this tab must not initiate another unattended recovery loop.
          if (reserveReturn()) {
            heading("Возвращаемся на страницу");
            status.textContent = "Вход подтверждён. Открываем страницу автоматически.";
            navigationTimer = setTimeout(() => {
              if (leaving) return;
              heading("Автоматический переход не завершён");
              status.textContent = "Переход не завершился. Нажмите «Продолжить» или повторите проверку. Сохранение и отправка не повторялись.";
              next.hidden = false;
              retry.hidden = false;
              if (signIn) signIn.hidden = false;
            }, 10000);
            location.reload();
          }
          return;
        }
        if (response.status !== 409 || body.code !== "authentication_recovery_continue") break;
      }
      throw new Error("recovery not confirmed");
    } catch {
      if (leaving) return;
      heading("Не удалось подтвердить вход");
      if (signIn) signIn.hidden = false;
      status.textContent = "Не удалось подтвердить вход. Повторите проверку или выполните новый вход по ссылке, если она доступна. Сохранение и отправка не повторялись.";
      retry.hidden = false;
    } finally {
      clearTimeout(timeout);
      pending = false;
      activeController = undefined;
    }
  }
  retry.addEventListener("click", recover);
  // Manual fallback remains available when storage is unavailable or a recent
  // automatic return was already attempted.
  // A response never supplies its destination; the current document is retained.
  next.addEventListener("click", () => { abandon(); location.reload(); });
  if (recentlyReturned()) {
    heading("Нужна повторная проверка");
    status.textContent = "Автоматический переход приостановлен, чтобы не повторять его по кругу. Нажмите «Повторить проверку» или войдите заново.";
    retry.hidden = false;
  } else {
    void recover();
  }
})();
