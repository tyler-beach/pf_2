// Literal requires are allow-listed host modules, not dynamic plugins.
const fs = require("fs");
const path = require("path");

module.exports = { fs, path };
