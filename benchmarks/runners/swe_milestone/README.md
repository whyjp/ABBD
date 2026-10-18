# Track B. SWE-Milestone

Not run. Commands are from DeepCommit-ai/SWE-Milestone @ `17a8f1593e172e26b36cea15e2b30fb9536c93f5`
(benchmark v1.0.2) and its README, read 2026-10-18.

## Facts verified

- Dataset: huggingface.co/datasets/DeepCommit-ai/SWE-Milestone-data, tag v1.0.2 =
  `3500478e740d42583277c2658aa7729eb36e10f4`. MIT. 7 repos, 98 graded milestones, 110 active DAG edges.
- All seven repos the spec names exist: go-zero (Go, 23 graded), Element Web (TS, 18), Apache Dubbo (Java, 12),
  Nushell (Rust, 13), ripgrep (Rust, 11), scikit-learn (Python, 12), Navidrome (Go, 9).
- DAG: `dependencies.csv` (`source_id,target_id,type,strength,rationale,confidence_score`). Default
  `e2e_config.yaml`: `early_unblock: true`, `ignore_weak_dependencies: true`.
- One persistent container per repo, workspace `/testbed`, no reset between milestones. The agent reads
  `/e2e_workspace/TASK_QUEUE.md` and `srs/<id>_SRS.md`, and submits by tagging `agent-impl-milestone_NNN`.
  Only milestones whose dependencies are met appear in the queue, so future requirements are not shown.
- Built-in agents: claude-code, codex, gemini-cli, openhands. Default per-repo timeout in the example
  config: 18000 s.
- Images: `hyd2apse/swe-milestone__<repo>__<milestone>:<version>`, linux/amd64, about 115 images. Agent
  container has an outbound allowlist (model APIs and package registries; GitHub and GitLab blocked).
- Metrics (harness/e2e/collect_results.py `calculate_score_reliable`): Recall = N_fixed / N_target;
  Precision = (N_fixed + 1) / (N_fixed + N_broken + 1); Score = 2PR / (P + R). A milestone is fully resolved
  when every F2P test passes and no P2P test regresses. Build failures score 0 and stay in the denominator.

## Setup (Linux Docker host)

```sh
git clone https://github.com/DeepCommit-ai/SWE-Milestone && cd SWE-Milestone && git checkout 17a8f15
uv sync
./scripts/pull_data.sh          # git-lfs clone of the HF dataset; pin tag v1.0.2
./scripts/pull_images.sh        # docker login first; ~115 images
cp trial_config.example.yaml trial_config.yaml
```

For groups B to E, copy `benchmarks/adapters/for_my_harness/swe_milestone_agent.py` into
`harness/e2e/agents/` and add one import line to the local `harness/e2e/agents/__init__.py`. Record that
diff in the run metadata. Group A runs the unmodified harness.

## Runs

`trial_config.yaml` per group and rep: `agent: claude-code` (A) or `claude-code-abbd` (B to E),
`model: <exact id>`, `agent_version: <pinned>`, `timeout: 18000`, `reasoning_effort: <same for all>`,
`trial_name: <group>_<model>_r<k>`. Repos: pilot = go-zero and Element Web (manifest).

```sh
ABBD_GROUP=E ABBD_BIN=/abs/bench/abbd-linux ABBD_PLUGIN=<cccv>/plugins/abbd \
  python scripts/run_all.py --config trial_config.yaml
./scripts/monitor.sh <trial_name>
python -m harness.e2e.collect_results --workspace-root <root> --trials <trial_name> --trial-type e2e
```

## Outputs to keep

The trial directory (including `trial_metadata.json` with the requested and installed agent versions) goes
to `results/raw/<run_id>/`. Official per-repo Score, Precision, Recall and Fully Resolved come from
`collect_results` unchanged. In-house metrics (cumulative preservation, regressions per step, impact hit and
miss, cost per milestone, 25/50/75/100% snapshots) are computed from the per-milestone evaluation records and
the submission tags, and stored under `in_house`. They are never merged into Score.
