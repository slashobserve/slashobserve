;(function () {
  var p
  try {
    p = localStorage.getItem("slashobserve-theme")
  } catch {
    p = null
  }
  var dark =
    p === "dark" ||
    (p !== "light" && window.matchMedia("(prefers-color-scheme: dark)").matches)
  document.documentElement.classList.toggle("dark", dark)
  document.documentElement.style.colorScheme = dark ? "dark" : "light"
})()
