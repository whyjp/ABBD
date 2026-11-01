<!-- abbd:begin -->
## ABBD blueprint

This repository keeps its architecture topology in `.abbd/`, drawn at `.abbd/blueprint/index.html`.

- Ask architecture questions of the blueprint first, before grepping: `abbd explain <ID|NAME>`, `abbd path <A> <B>`, `abbd impact <IDS>`.
- After changing code, run `abbd sync`, put `abbd diff` in the PR description, and commit `.abbd/` with the code.
- The /abbd skill says how to read, query and record the topology.
<!-- abbd:end -->
