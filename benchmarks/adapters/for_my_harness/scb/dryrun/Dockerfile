# Dry-run layer over the real environment image: adds the fake `claude` and one problem's reference
# solutions and specs. Used only by runners/slop_code_bench/dryrun.sh; never for scored runs.
ARG BASE
FROM ${BASE}
COPY fake_claude.py /opt/fake/claude
COPY solutions/ /opt/fake/solutions/
RUN chmod 755 /opt/fake/claude && chmod -R a+rX /opt/fake
