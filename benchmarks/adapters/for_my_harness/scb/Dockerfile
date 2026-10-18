# UNTESTED. One environment image for every SlopCodeBench group, so the base never differs between groups.
# ABBD is present for all groups but on PATH only for C, D, E (via the agent config's env).
# Build context: a staging dir holding abbd-linux and plugins/{pstack,C,D,E}.
# Open item: slop-code-bench's claude_code docker.j2 switches to `USER agent`; confirm whether the runner
# creates that user in an intermediate image or expects it in the base.
FROM ghcr.io/astral-sh/uv:python3.12-trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
COPY abbd-linux /opt/abbd/bin/abbd
COPY plugins/ /opt/bench-plugins/
RUN chmod 755 /opt/abbd/bin/abbd && chmod -R a+rX /opt/bench-plugins
