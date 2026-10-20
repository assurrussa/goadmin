"use strict";
(() => {
  const status = document.getElementById("auth-recovery-status");
  const retry = document.getElementById("auth-recovery-retry");
  const next = document.getElementById("auth-recovery-continue");
  if (!status || !retry || !next) return;
  let pending = false;
  async function recover() {
    if (pending) return;
    pending = true;
    retry.hidden = true;
    next.hidden = true;
    status.textContent = "Восстанавливаем подтверждение доступа. Сохранение и отправка не повторяются.";
    const controller = new AbortController();
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
          status.textContent = "Вход подтверждён. Нажмите «Продолжить», чтобы открыть страницу. Сохранение и отправка не повторялись.";
          next.hidden = false;
          return;
        }
        if (response.status !== 409 || body.code !== "authentication_recovery_continue") break;
      }
      throw new Error("recovery not confirmed");
    } catch {
      status.textContent = "Не удалось подтвердить вход. Повторите проверку или выполните новый вход по ссылке, если она доступна. Сохранение и отправка не повторялись.";
      retry.hidden = false;
    } finally {
      clearTimeout(timeout);
      pending = false;
    }
  }
  retry.addEventListener("click", recover);
  // Explicit safe GET continuation avoids unattended recovery/reload loops.
  // A response never supplies its destination; the current document is retained.
  next.addEventListener("click", () => location.reload());
  void recover();
})();
