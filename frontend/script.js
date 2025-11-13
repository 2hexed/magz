/* ============================================================
   Magz — Library Script
   ============================================================ */

"use strict";

/* ── Helpers ─────────────────────────────────────────────── */
function escapeHtml(str) {
  const d = document.createElement("div");
  d.textContent = String(str || "");
  return d.innerHTML;
}

function debounce(fn, ms) {
  let timer;
  return function (...args) {
    clearTimeout(timer);
    timer = setTimeout(() => fn.apply(this, args), ms);
  };
}

/* ── Skeleton loaders ────────────────────────────────────── */
function renderSkeletons(count) {
  const container = document.getElementById("library");
  const frag = document.createDocumentFragment();
  for (let i = 0; i < count; i++) {
    const card = document.createElement("div");
    card.className = "skeleton-card";
    card.setAttribute("aria-hidden", "true");
    card.innerHTML = `
      <div class="skeleton-cover"></div>
      <div class="skeleton-info">
        <div class="skeleton-line"></div>
        <div class="skeleton-line short"></div>
      </div>
    `;
    frag.appendChild(card);
  }
  container.innerHTML = "";
  container.appendChild(frag);
}

/* ── State views ─────────────────────────────────────────── */
function showError(message) {
  const container = document.getElementById("library");
  container.innerHTML = `
    <div class="state-container">
      <div class="state-icon" aria-hidden="true">⚠️</div>
      <p class="state-title">Couldn't load library</p>
      <p class="state-sub">${escapeHtml(message)}</p>
      <button class="btn-retry" onclick="loadLibrary()">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2.5"
          stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true" focusable="false">
          <polyline points="23 4 23 10 17 10"/>
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
        </svg>
        Try again
      </button>
    </div>
  `;
}

function showEmpty() {
  const container = document.getElementById("library");
  container.innerHTML = `
    <div class="state-container">
      <div class="state-icon" aria-hidden="true">📚</div>
      <p class="state-title">Library is empty</p>
      <p class="state-sub">Add magazines, comics or books to your configured library paths and refresh.</p>
    </div>
  `;
}

function showNoResults(term) {
  const container = document.getElementById("library");
  // Remove existing no-results node only
  const existing = container.querySelector(".no-results-state");
  if (existing) existing.remove();

  const div = document.createElement("div");
  div.className = "state-container no-results-state";
  div.setAttribute("role", "status");
  div.innerHTML = `
    <div class="state-icon" aria-hidden="true">🔍</div>
    <p class="state-title">No results</p>
    <p class="state-sub">Nothing matched "<strong>${escapeHtml(term)}</strong>". Try a different search.</p>
  `;
  container.appendChild(div);
}

/* ── Card builder ────────────────────────────────────────── */
function createCard(mag) {
  const FALLBACK_SVG = `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 400">
      <rect width="300" height="400" fill="#e8e3da"/>
      <text x="150" y="200" text-anchor="middle" fill="#b0a898"
        font-size="16" font-family="serif">No Cover</text>
    </svg>`,
  )}`;

  const article = document.createElement("article");
  article.className = "mag-item";
  article.setAttribute("tabindex", "0");
  article.setAttribute("role", "listitem");
  article.setAttribute(
    "aria-label",
    mag.title + (mag.category ? ", " + mag.category : ""),
  );

  const src = mag.coverData || FALLBACK_SVG;

  article.innerHTML = `
    <div class="cover-wrap" id="cover-${mag.id}">
      <img
        src="${src}"
        alt="${escapeHtml(mag.title)}"
        loading="lazy"
        decoding="async"
      />
      <span class="read-btn" aria-hidden="true">Read</span>
    </div>
    <div class="info">
      <h3>${escapeHtml(mag.title)}</h3>
      <p class="cat">${escapeHtml(mag.category || "")}</p>
    </div>
  `;

  /* Image load handling — skeleton → reveal */
  const wrap = article.querySelector(".cover-wrap");
  const img = article.querySelector("img");

  function onImgLoad() {
    img.classList.add("img-loaded");
    wrap.classList.add("loaded");
  }

  if (img.complete && img.naturalWidth > 0) {
    onImgLoad();
  } else {
    img.addEventListener("load", onImgLoad);
    img.addEventListener("error", () => {
      img.src = FALLBACK_SVG;
      onImgLoad();
    });
  }

  /* Navigation */
  function navigate() {
    location.href = "/viewer.html?id=" + encodeURIComponent(mag.id);
  }

  article.addEventListener("click", navigate);
  article.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      navigate();
    }
  });

  return article;
}

/* ── Main load ───────────────────────────────────────────── */
async function loadLibrary() {
  /* Show ~12 skeleton cards while loading */
  renderSkeletons(12);

  try {
    const res = await fetch("/api/library");
    if (!res.ok) throw new Error("Server responded " + res.status);
    const mags = await res.json();

    const container = document.getElementById("library");
    container.innerHTML = "";

    if (!mags || mags.length === 0) {
      showEmpty();
      return;
    }

    /* Badge */
    const badge = document.getElementById("libCount");
    badge.textContent =
      mags.length + (mags.length === 1 ? " title" : " titles");
    badge.style.display = "inline-flex";

    /* Render cards */
    const frag = document.createDocumentFragment();
    mags.forEach((mag) => frag.appendChild(createCard(mag)));
    container.appendChild(frag);

    /* Store for search filtering */
    container._allMags = mags;

    document.title = "Magz (" + mags.length + ")";
  } catch (err) {
    console.error("Library load failed:", err);
    showError(err.message || "Unknown error");
  }
}

/* ── Search / filter ─────────────────────────────────────── */
function filterLibrary(term) {
  const container = document.getElementById("library");
  const items = container.querySelectorAll(".mag-item");
  const q = term.trim().toLowerCase();

  /* Remove previous no-results message */
  const prev = container.querySelector(".no-results-state");
  if (prev) prev.remove();

  if (!q) {
    items.forEach((el) => (el.style.display = ""));
    return;
  }

  let visible = 0;
  items.forEach((el) => {
    const title = el.querySelector("h3")?.textContent.toLowerCase() || "";
    const cat = el.querySelector(".cat")?.textContent.toLowerCase() || "";
    const match = title.includes(q) || cat.includes(q);
    el.style.display = match ? "" : "none";
    if (match) visible++;
  });

  if (visible === 0) showNoResults(term);
}

/* ── Bootstrap ───────────────────────────────────────────── */
document.addEventListener("DOMContentLoaded", () => {
  loadLibrary();

  const searchInput = document.getElementById("searchInput");
  const searchBox = document.getElementById("searchBox");
  const clearBtn = document.getElementById("searchClear");

  const debouncedFilter = debounce((val) => filterLibrary(val), 220);

  searchInput.addEventListener("input", (e) => {
    const val = e.target.value;
    searchBox.classList.toggle("has-value", val.length > 0);
    debouncedFilter(val);
  });

  clearBtn.addEventListener("click", () => {
    searchInput.value = "";
    searchBox.classList.remove("has-value");
    filterLibrary("");
    searchInput.focus();
  });

  searchInput.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      searchInput.value = "";
      searchBox.classList.remove("has-value");
      filterLibrary("");
      searchInput.blur();
    }
  });

  /* Ctrl/Cmd+K → focus search */
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "k") {
      e.preventDefault();
      searchInput.focus();
      searchInput.select();
    }
  });
});
