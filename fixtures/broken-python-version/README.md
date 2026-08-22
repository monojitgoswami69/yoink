# broken-python-version (Yoink demo fixture)

A deterministic, LLM-independent demonstration of Yoink's repair loop.

## What's intentionally broken

`pyproject.toml` declares `requires-python = ">=3.13"`, but Yoink's
generator emits a `python:3.12-slim` base image by default. So the build
fails during `pip install` with:

```
ERROR: Package 'demo' requires a different Python: 3.12.0 not in '<requires-python>' (>=3.13).
```

## What Yoink does

1. Detects Flask (Python).
2. Generates a Dockerfile + compose (`python:3.12-slim`).
3. Build fails (version mismatch).
4. The **deterministic fixer** (no LLM) bumps the base image to
   `python:3.13-slim`.
5. Rebuild succeeds.
6. Runtime + HTTP verification pass → `http://localhost:5000`.

## Why it's a reliable demo

- Deterministic root cause (no Gemini/provider randomness).
- No external secrets.
- No internet after `python:3.13-sim` + `flask` are cached.
- Repeatable.

## Run

```
yoink init ./fixtures/broken-python-version --name broken-demo
# → Build failed → deterministic fix: bumped Python base to 3.13 → rebuild → Running
#   http://localhost:5000
```
