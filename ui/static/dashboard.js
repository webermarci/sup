document.addEventListener("DOMContentLoaded", () => {
  function displayValue(value) {
    return typeof value === "object" && value !== null
      ? JSON.stringify(value)
      : String(value ?? "");
  }

  function getStateElement(name) {
    return document.querySelector(
      `.state-val[data-name="${CSS.escape(name)}"]`,
    );
  }

  function handleUpdate(evt) {
    const name = evt?.name;
    if (!name) return;

    const el = getStateElement(name);
    if (!el) return;

    const textEl = el.querySelector(".state-text");
    const text = displayValue(evt.value);

    if (textEl) textEl.textContent = text;
    el.dataset.value = text;
    el.title = text;
  }

  const sse = new EventSource("/api/events");

  sse.addEventListener("update", (e) => {
    try {
      handleUpdate(JSON.parse(e.data));
    } catch (err) {
      console.error("sse update parse error", err);
    }
  });

  sse.addEventListener("error", (e) => {
    console.error("sse connection error", e);
  });

  window.addEventListener("beforeunload", () => {
    sse.close();
  });
});
