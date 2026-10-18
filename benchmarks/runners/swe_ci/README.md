# Track D. SWE-CI (and SkillsBench)

Not run and not adapted in phase 1.

## SWE-CI facts verified (2026-10-18)

- Repo: github.com/SKYLENAGE-AI/SWE-CI @ `b2a0620f0168a5a89681be7919a98d9a49ab22af`. Its README clones
  `Loong-Chan/SWE-CI`, which returns 404. Paper: arxiv.org/pdf/2603.03823.
  Dataset: huggingface.co/datasets/skylenage-ai/SWE-CI (about 50 GB).
- 100 tasks, each a base commit and a reference commit. A CI loop of up to `max_epoch = 20` iterations
  with an Architect agent (reads test failures, writes requirements) and a Programmer agent.
- Official metric, Average Normalized Change (ANC), README verbatim in structure:
  `a_i = (p_i - p_0) / (p_* - p_0)` if `p_i >= p_0`, else `(p_i - p_0) / p_0`;
  `ANC = (1 / (M * N)) * sum_j sum_i a_i^(j)`, with p_i tests passed at iteration i, p_0 at start, p_* in the
  reference, N max iterations, M tasks.
- Agents: `iflow` and `opencode` only (`config.toml agent_name`). No Claude Code adapter. README: "designed to
  run on Linux with iFlow CLI". Takes an OpenAI-compatible `--api_key --base_url --model_name`.
- Full run estimate in the README: about 48 hours on 32 cores, 64 GB RAM, 16 workers. Docker required.
- Leaderboard: an image in the repo (`docs/result.png`), not transcribed.

## Commands (from the README)

```sh
PYTHONPATH=src python -m swe_ci.download
PYTHONPATH=src python -m swe_ci.evaluate --experiment_name <name> --api_key <key> --base_url <url> --model_name <model>
PYTHONPATH=src python -m swe_ci.summarize
```

## What it would take

A new `claude_code` agent adapter in `src/swe_ci/benchmark/agents/` for both roles, written against that
directory's interface. That is more than a minimal adapter, and the two-agent loop is a different protocol
from ABBD's single-agent turn. Any result would be labelled a partial evaluation with a custom adapter.
Recommendation: defer until Tracks A to C have pilot data.

## SkillsBench facts verified (2026-10-18)

- Repo: github.com/benchflow-ai/skillsbench @ `f860ecf02ef53412a030f8661c83d644b739ff01`. Site: skillsbench.ai
  (skillsbench@1.1, 87 tasks, up to 3 trials, recomputed 2026-07-16). Paper: arxiv.org/abs/2602.12670.
- The current repo runs on BenchFlow (`uv tool install benchflow`, `bench eval run`), not Harbor. Only the
  paper says Harbor. Task counts differ between paper (84 to 86), poster (84) and site and repo (87).
- Software-engineering subset: 16 tasks (`category: software-engineering`).
- Skills per task under `environment/skills/<skill>/SKILL.md`; `--skill-mode with-skill --skills-dir <dir>`.
- Default sandbox Modal (`MODAL_TOKEN_ID`, `MODAL_TOKEN_SECRET`); `--sandbox docker` runs locally.

SkillsBench measures whether an agent uses task-specific skills it is handed. ABBD's skills are a
development workflow, not task knowledge, so a SkillsBench run would test a different claim. It is optional
and not scaffolded further.
