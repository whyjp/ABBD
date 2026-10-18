# Environment base image for every SlopCodeBench group (A to E), so the base never differs between groups.
# slop-code-bench builds two layers on top of it: setup_base.docker.j2 (apt tools, node, rust, user `agent`,
# HOME=/tmp/agent_home) and claude_code/docker.j2 (npm install of the pinned Claude Code).
# ABBD is present for every group but reaches only C/D/E: their agent config sets `binary` to claude-abbd,
# which puts /opt/abbd/bin on PATH. A and B call `claude` directly and never see it on PATH.
# Build context: the staging dir that runners/slop_code_bench/setup.sh assembles.
FROM ghcr.io/astral-sh/uv:python3.12-trixie-slim
COPY abbd-linux /opt/abbd/bin/abbd
COPY claude-abbd /opt/abbd/launch/claude-abbd
COPY plugins/ /opt/bench-plugins/
RUN chmod 755 /opt/abbd/bin/abbd /opt/abbd/launch/claude-abbd && chmod -R a+rX /opt/bench-plugins /opt/abbd
