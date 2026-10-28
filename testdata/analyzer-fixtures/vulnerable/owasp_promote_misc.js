// Dynamic non-literal require for untrusted plugin boundary.
function loadPlugin(name) {
  return require(name);
}
