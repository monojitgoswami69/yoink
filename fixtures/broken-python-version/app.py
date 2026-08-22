import os
from flask import Flask

app = Flask(__name__)


@app.get("/")
def index():
    return {"ok": True, "service": "broken-demo", "python_required": ">=3.13"}


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", "5000")))
