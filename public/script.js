// Debounce function for search
function debounce(func, wait) {
  let timeout;
  return function executedFunction(...args) {
    const later = () => {
      clearTimeout(timeout);
      func(...args);
    };
    clearTimeout(timeout);
    timeout = setTimeout(later, wait);
  };
}

// Show loading state
function showLoading() {
  const container = document.getElementById("library");
  container.innerHTML = `
    <div style="grid-column: 1/-1; text-align: center; padding: 4rem;">
      <div style="font-size: 2rem; margin-bottom: 1rem;">📚</div>
      <p style="color: var(--fg-muted);">Loading your library...</p>
    </div>
  `;
}

// Show error state
function showError(message) {
  const container = document.getElementById("library");
  container.innerHTML = `
    <div style="grid-column: 1/-1; text-align: center; padding: 4rem;">
      <div style="font-size: 2rem; margin-bottom: 1rem;">⚠️</div>
      <p style="color: var(--fg-muted);">${message}</p>
      <button onclick="loadLibrary()" style="margin-top: 1rem;">Retry</button>
    </div>
  `;
}

// Show empty state
function showEmpty() {
  const container = document.getElementById("library");
  container.innerHTML = `
    <div style="grid-column: 1/-1; text-align: center; padding: 4rem;">
      <div style="font-size: 2rem; margin-bottom: 1rem;">📖</div>
      <p style="color: var(--fg-muted);">No magazines found</p>
      <p style="color: var(--fg-muted); font-size: 0.9rem; margin-top: 0.5rem;">
        Check your library paths in the configuration
      </p>
    </div>
  `;
}

// Create magazine card element
function createMagCard(mag) {
  const div = document.createElement("div");
  div.className = "mag-item";
  div.setAttribute("tabindex", "0");
  div.setAttribute("role", "button");
  div.setAttribute("aria-label", `Read ${mag.title}`);

  // Handle missing cover data
  const coverSrc =
    mag.coverData ||
    "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 300 400'%3E%3Crect width='300' height='400' fill='%23e0e0e0'/%3E%3Ctext x='50%25' y='50%25' text-anchor='middle' fill='%23999' font-size='20' font-family='sans-serif'%3ENo Cover%3C/text%3E%3C/svg%3E";

  div.innerHTML = `
    <div class="cover-wrap">
      <img 
        src="${coverSrc}" 
        alt="${escapeHtml(mag.title)}" 
        loading="lazy"
        onerror="this.src='data:image/svg+xml,%3Csvg xmlns=\\'http://www.w3.org/2000/svg\\' viewBox=\\'0 0 300 400\\'%3E%3Crect width=\\'300\\' height=\\'400\\' fill=\\'%23e0e0e0\\'/%3E%3Ctext x=\\'50%25\\' y=\\'50%25\\' text-anchor=\\'middle\\' fill=\\'%23999\\' font-size=\\'20\\' font-family=\\'sans-serif\\'%3ENo Cover%3C/text%3E%3C/svg%3E'"
      />
    </div>
    <div class="info">
      <h3>${escapeHtml(mag.title)}</h3>
      <p class="cat">${escapeHtml(mag.category)}</p>
    </div>
  `;

  // Handle click and keyboard navigation
  const navigate = () => {
    location.href = `/viewer.html?id=${mag.id}`;
  };

  div.onclick = navigate;
  div.onkeydown = (e) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      navigate();
    }
  };

  return div;
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
}

// Load and display library
async function loadLibrary() {
  showLoading();

  try {
    const res = await fetch("/api/library");

    if (!res.ok) {
      throw new Error(`Server error: ${res.status}`);
    }

    const mags = await res.json();
    const container = document.getElementById("library");

    if (!mags || mags.length === 0) {
      showEmpty();
      return;
    }

    container.innerHTML = "";

    // Create fragment for better performance
    const fragment = document.createDocumentFragment();
    mags.forEach((mag) => {
      fragment.appendChild(createMagCard(mag));
    });

    container.appendChild(fragment);

    // Update document title with count
    document.title = `Magz Library (${mags.length})`;
  } catch (err) {
    console.error("Failed to load library:", err);
    showError("Failed to load library. Please try again.");
  }
}

// Filter magazines based on search term
function filterMagazines(term) {
  const lowerTerm = term.toLowerCase().trim();
  const items = document.querySelectorAll(".mag-item");
  let visibleCount = 0;

  items.forEach((item) => {
    const title = item.querySelector("h3").textContent.toLowerCase();
    const category = item.querySelector(".cat").textContent.toLowerCase();
    const matches = title.includes(lowerTerm) || category.includes(lowerTerm);

    item.style.display = matches ? "" : "none";
    if (matches) visibleCount++;
  });

  // Show "no results" message if nothing matches
  const container = document.getElementById("library");
  let noResults = container.querySelector(".no-results");

  if (visibleCount === 0 && term.length > 0) {
    if (!noResults) {
      noResults = document.createElement("div");
      noResults.className = "no-results";
      noResults.style.cssText =
        "grid-column: 1/-1; text-align: center; padding: 4rem;";
      noResults.innerHTML = `
        <div style="font-size: 2rem; margin-bottom: 1rem;">🔍</div>
        <p style="color: var(--fg-muted);">No results found for "${escapeHtml(term)}"</p>
      `;
      container.appendChild(noResults);
    }
  } else if (noResults) {
    noResults.remove();
  }
}

// Initialize on page load
document.addEventListener("DOMContentLoaded", () => {
  loadLibrary();

  // Setup search with debouncing
  const searchInput = document.getElementById("searchInput");
  const debouncedFilter = debounce((term) => filterMagazines(term), 300);

  searchInput.addEventListener("input", (e) => {
    debouncedFilter(e.target.value);
  });

  // Clear search with Escape key
  searchInput.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      searchInput.value = "";
      filterMagazines("");
      searchInput.blur();
    }
  });

  // Focus search with Ctrl/Cmd + K
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "k") {
      e.preventDefault();
      searchInput.focus();
    }
  });
});
