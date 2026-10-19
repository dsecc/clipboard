(() => {
  "use strict";
  const $ = (id) => document.getElementById(id);
  const listEl = $("list"), emptyEl = $("empty"), errEl = $("error"), input = $("input");
  const statusEl = $("status");
  const entries = new Map(); // id -> entry
  let device = "";

  // ---- storage (may be unavailable) ----
  const store = {
    get(k) { try { return localStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch {} },
  };

  // ---- helpers ----
  function showError(msg) {
    errEl.textContent = msg; errEl.hidden = false;
    clearTimeout(showError.t); showError.t = setTimeout(() => (errEl.hidden = true), 6000);
  }
  function ago(iso) {
    const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
    if (s < 45) return "ahora";
    const m = Math.round(s / 60);
    if (m < 60) return `hace ${m} min`;
    return `hace ${Math.round(m / 60)} h`;
  }

  async function copyText(text) {
    try { await navigator.clipboard.writeText(text); return true; } catch {}
    const ta = document.createElement("textarea");
    ta.value = text; ta.style.cssText = "position:fixed;top:0;left:0;opacity:0";
    document.body.appendChild(ta); ta.focus(); ta.select(); ta.setSelectionRange(0, text.length);
    let ok = false; try { ok = document.execCommand("copy"); } catch {}
    ta.remove(); return ok;
  }
  // Safari only allows clipboard.write inside the tap and only for PNG, so the
  // write starts right away with a promise that fetches the image and converts it.
  function copyImage(id) {
    const png = (async () => {
      const res = await fetch(`/api/images/${id}`);
      if (!res.ok) throw new Error("no se pudo bajar la imagen");
      const blob = await res.blob();
      if (blob.type === "image/png") return blob;
      const bmp = await createImageBitmap(blob);
      const canvas = document.createElement("canvas");
      canvas.width = bmp.width; canvas.height = bmp.height;
      canvas.getContext("2d").drawImage(bmp, 0, 0);
      return new Promise((ok, fail) => canvas.toBlob((b) => (b ? ok(b) : fail(new Error("conversión fallida"))), "image/png"));
    })();
    return navigator.clipboard.write([new ClipboardItem({ "image/png": png })]);
  }

  // ---- rendering ----
  function render(e) {
    const wrap = document.createElement("div");
    wrap.className = "entry"; wrap.dataset.id = e.id;
    const meta = document.createElement("div"); meta.className = "meta";
    const who = document.createElement("span"); who.textContent = e.device;
    const when = document.createElement("span"); when.className = "when"; when.dataset.t = e.createdAt; when.textContent = ago(e.createdAt);
    const del = document.createElement("button"); del.className = "del"; del.type = "button"; del.textContent = "Borrar";
    del.setAttribute("aria-label", "Borrar este mensaje");
    del.addEventListener("click", (ev) => { ev.stopPropagation(); remove(e.id); });
    meta.append(who, when, del);

    const bub = document.createElement("div"); bub.className = "bubble"; bub.tabIndex = 0; bub.setAttribute("role", "button");
    if (e.kind === "image") {
      const img = document.createElement("img"); img.src = `/api/images/${e.id}`; img.alt = "Imagen"; img.loading = "lazy";
      img.addEventListener("load", () => { if (stickToBottomIfNear.want) listEl.scrollTop = listEl.scrollHeight; });
      bub.append(img);
    } else {
      bub.append(document.createTextNode(e.text)); // never innerHTML
      if (e.text.includes("\n")) bub.classList.add("mono");
    }
    const ok = document.createElement("span"); ok.className = "ok"; ok.textContent = "✓ Copiado"; bub.append(ok);
    const act = async () => {
      if (String(getSelection())) return; // let the user select part of the text instead
      try {
        if (e.kind === "image") await copyImage(e.id); else if (!(await copyText(e.text))) throw new Error();
        bub.classList.add("copied"); setTimeout(() => bub.classList.remove("copied"), 1400);
      } catch { showError("No pude copiar. Mantené apretado el texto para copiarlo a mano."); }
    };
    bub.addEventListener("click", act);
    bub.addEventListener("keydown", (ev) => { if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); act(); } });
    wrap.append(meta, bub);
    return wrap;
  }
  function nearBottom() { return listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < 120; }
  function stickToBottomIfNear() {}
  function refreshEmpty() { emptyEl.hidden = entries.size > 0; }

  function add(e) {
    if (entries.has(e.id)) return;
    const stick = nearBottom(); entries.set(e.id, e);
    const node = render(e);
    let placed = false; // keep chronological order
    for (const n of listEl.children) {
      const o = entries.get(n.dataset.id);
      if (o && new Date(o.createdAt) > new Date(e.createdAt)) { listEl.insertBefore(node, n); placed = true; break; }
    }
    if (!placed) listEl.append(node);
    stickToBottomIfNear.want = stick;
    if (stick) listEl.scrollTop = listEl.scrollHeight;
    refreshEmpty();
  }
  function drop(id) {
    entries.delete(id);
    listEl.querySelector(`[data-id="${CSS.escape(id)}"]`)?.remove();
    refreshEmpty();
  }

  // ---- server ----
  async function api(path, opts) {
    const res = await fetch(path, opts);
    if (!res.ok) {
      let msg = "Algo salió mal.";
      try { msg = (await res.json()).error || msg; } catch {}
      throw new Error(msg);
    }
    return res.status === 204 ? null : res.json();
  }
  async function resync() {
    const list = await api("/api/entries");
    const ids = new Set(list.map((e) => e.id));
    for (const id of [...entries.keys()]) if (!ids.has(id)) drop(id);
    list.forEach(add);
    listEl.scrollTop = listEl.scrollHeight;
  }
  async function remove(id) {
    try { await api(`/api/entries/${id}`, { method: "DELETE" }); drop(id); }
    catch (err) { if (/no existe/.test(err.message)) drop(id); else showError(err.message); }
  }
  async function sendText(text) {
    try {
      const e = await api("/api/entries", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ text, device }) });
      add(e);
    } catch (err) { showError(err.message); throw err; }
  }
  async function sendImage(file) {
    try {
      const e = await api("/api/images", { method: "POST", headers: { "Content-Type": file.type, "X-Device": device }, body: file });
      add(e);
    } catch (err) { showError(err.message); }
  }

  let es = null, retry = 0, retryTimer = 0;
  const setStatus = (cls, txt) => { statusEl.className = "status " + cls; statusEl.lastElementChild.textContent = txt; };
  function connect() {
    clearTimeout(retryTimer);
    if (es) es.close();
    es = new EventSource("/api/events");
    const mine = es;
    mine.onopen = () => { retry = 0; setStatus("on", "conectado"); resync().catch((e) => showError(e.message)); };
    mine.onerror = () => {
      setStatus("off", "reconectando…");
      // after a 502 from the proxy (server restarting) the browser gives up for good, so retry by hand
      if (mine.readyState === EventSource.CLOSED) {
        retry = Math.min(retry + 1, 6);
        retryTimer = setTimeout(connect, Math.min(1000 * 2 ** retry, 15000));
      }
    };
    mine.addEventListener("added", (ev) => add(JSON.parse(ev.data)));
    mine.addEventListener("deleted", (ev) => drop(JSON.parse(ev.data).id));
  }
  // iOS drops the stream when the phone sleeps; check again whenever the page comes back
  function checkConnection() {
    if (!device) return; // still asking for the device name
    if (!es || es.readyState === EventSource.CLOSED) connect();
    else if (es.readyState === EventSource.OPEN) resync().catch(() => {});
  }
  document.addEventListener("visibilitychange", () => { if (!document.hidden) checkConnection(); });
  window.addEventListener("online", checkConnection);
  window.addEventListener("pageshow", checkConnection);

  // ---- composer ----
  function autosize() { input.style.height = "auto"; input.style.height = Math.min(input.scrollHeight, 160) + "px"; }
  input.addEventListener("input", autosize);
  input.addEventListener("keydown", async (ev) => {
    if (ev.key === "Enter" && !ev.shiftKey && !ev.isComposing) {
      ev.preventDefault();
      const text = input.value;
      if (!text.trim()) return;
      input.value = ""; autosize();
      try { await sendText(text); } catch { input.value = text; autosize(); }
    }
  });
  input.addEventListener("paste", (ev) => {
    const item = [...(ev.clipboardData?.items || [])].find((i) => i.type.startsWith("image/"));
    if (item) { ev.preventDefault(); sendImage(item.getAsFile()); }
  });
  $("composer").addEventListener("submit", (ev) => ev.preventDefault());
  $("file").addEventListener("change", (ev) => { const f = ev.target.files[0]; ev.target.value = ""; if (f) sendImage(f); });

  // ---- iOS keyboard: keep composer visible ----
  if (window.visualViewport) {
    const fit = () => { $("app").style.height = visualViewport.height + "px"; window.scrollTo(0, 0); };
    visualViewport.addEventListener("resize", fit); fit();
  }

  // ---- device name ----
  function guessName() {
    const ua = navigator.userAgent;
    if (/iPhone/.test(ua)) return "iPhone";
    if (/iPad/.test(ua)) return "iPad";
    if (/Android/.test(ua)) return "Android";
    return "Navegador web";
  }
  function start() {
    $("nameDialog").hidden = true; connect();
    setInterval(() => document.querySelectorAll(".when").forEach((n) => (n.textContent = ago(n.dataset.t))), 30000);
  }
  device = store.get("device") || "";
  if (device) start();
  else {
    $("nameDialog").hidden = false; $("nameInput").value = guessName();
    $("nameForm").addEventListener("submit", (ev) => {
      ev.preventDefault();
      device = $("nameInput").value.trim() || guessName(); store.set("device", device); start();
    });
  }
})();
