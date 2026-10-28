const { exec } = require("child_process");
exec("ls " + process.argv[2]);
fetch(process.argv[2]);
axios.get(process.argv[2]);
const apiKey = "sk_live_1234567890abcdef";
