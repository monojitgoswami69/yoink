const fs = require("fs");
const config = JSON.parse(fs.readFileSync("build-config.json", "utf8"));

if (process.env.BUILD_PROFILE !== config.requiredProfile) {
  throw new Error(
    `build profile mismatch: repository requires ${config.requiredProfile}`
  );
}

fs.writeFileSync("build-output.txt", "fixture built\n");
console.log("fixture build checks passed");
