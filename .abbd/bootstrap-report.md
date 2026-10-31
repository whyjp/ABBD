# Bootstrap report

Mode: code-only.
Extraction script: `go run .abbd/extractor/extract.go`. Three runs, the last with reseeded hash seeds, printed identical output (sha256 df982608a6564863).

## Design unit candidates

- CFG-0001 plugin and marketplace manifest (EXTRACTED), 2 anchor(s), signature `marketplace abbd {abbd=./plugins/abbd}; plugin abbd`, purpose INFERRED: publishes ABBD as a Claude Code plugin from this repository
- CFG-0002 project.yaml (EXTRACTED), 3 anchor(s), signature `.abbd/project.yaml: extract,verify`, purpose INFERRED: records the trusted extraction and verify argv
- CFG-0003 hook registration (EXTRACTED), 2 anchor(s), signature `PostToolUse[Write|Edit|NotebookEdit] -> abbd hook PostToolUse; PreToolUse[Bash|PowerShell|Write|Edit|NotebookEdit|WebFetch|mcp__.*] -> abbd hook PreToolUse; SessionStart -> abbd hook SessionStart; Stop -> abbd hook Stop; UserPromptSubmit -> abbd hook UserPromptSubmit`, purpose INFERRED: registers ABBD's hooks with Claude Code
- DATA-0001 extraction JSON (EXTRACTED), 3 anchor(s), signature `extraction json: relations{from,kind,to},units{anchors{path,symbol},key,kind,name,purpose,responsibility,signature}`, purpose INFERRED: is what every project extraction script and supplement must print
- DATA-0002 unit files (EXTRACTED), 3 anchor(s), signature `.abbd/{architecture,features}/ID.md frontmatter: absent,anchors{path,symbol},id,kind,name{confirmation,origin,rejected,value,verification},purpose{confirmation,origin,rejected,value,verification},relations{confirmation,kind,origin,to},responsibility{confirmation,origin,rejected,value,verification},signature{confirmation,origin,rejected,value,verification}`, purpose INFERRED: is the actual topology as reviewable files
- DATA-0003 generated files (EXTRACTED), 3 anchor(s), signature `.abbd/generated/{confirm-queue.html,explorer.html,graph.json}; graph.json {confirm_queue,drift{Detail,Effect,Key},edges{Class,D,Title},nodes{Class,Guess,ID,Kind,Name,R,Title,X,Y},phase,units{Anchors,Design,ID,Kind,Name,Purpose{Badge,Rejected,Value},Responsibility{Badge,Rejected,Value},Signature},watermark{inputs_hash,intended_revision}}`, purpose INFERRED: lets tools and humans read the topology without the CLI
- DATA-0004 decision records (EXTRACTED), 3 anchor(s), signature `.abbd/decisions/ADR-*.md frontmatter: confirmation,decision,grade,id,origin,question,reason,subject,turn`, purpose INFERRED: is the record of each human decision on a grade C change
- DATA-0005 intended topology (EXTRACTED), 3 anchor(s), signature `.abbd/intended.yaml: relations{from,kind,to},revision,units{external_consumers,id,kind,signature}`, purpose INFERRED: is the human-approved topology every gate compares against
- DATA-0006 verification runs (EXTRACTED), 3 anchor(s), signature `.abbd/verification/runs/run-*.yaml: command,exit_code,id,output,result,tree,turn,why`, purpose INFERRED: is the evidence that a turn's verification passed on its tree
- DATA-0007 change records (EXTRACTED), 3 anchor(s), signature `.abbd/changes/CHG-*.md frontmatter: approvals{adr,item},baseline{extractor,fingerprint,revision},id,impact{by,unit,why},intent_delta{changes,units},questions,rejections{adr,item,reason},report{declared_not_done,done_not_declared,gate,notes,why},requirement,seeds,session,state,verification`, purpose INFERRED: is the record of each turn
- DATA-0008 requirements (EXTRACTED), 2 anchor(s), signature `.abbd/requirements/ID.md frontmatter: id,origin,turn`, purpose INFERRED: keeps each user request verbatim
- FEAT-0001 /abbd-mode (EXTRACTED), 3 anchor(s), purpose INFERRED: runs a pstack playbook inside a gated ABBD turn
- FEAT-0002 /abbd-setup (EXTRACTED), 2 anchor(s), purpose INFERRED: bootstraps ABBD on a repository without asking the human
- IF-0001 Claude Code hook protocol (EXTRACTED), 3 anchor(s), signature `abbd hook {PostToolUse,PreToolUse,SessionStart,Stop,UserPromptSubmit}; stdin {cwd,session_id,stop_hook_active,tool_input{command,file_path,notebook_path},tool_name}; stdout {decision,hookSpecificOutput{additionalContext,hookEventName,permissionDecision,permissionDecisionReason},reason}`, purpose INFERRED: is how Claude Code hands ABBD each session event
- IF-0002 abbd command line (EXTRACTED), 3 anchor(s), signature `align[all,confirm,exclude,root] pos{0}; approve[root] pos{1}; confirm[list,reject,root,value] pos{0,1}; couple[root] pos{1}; hook[root] sub{*} pos{0}; ingest[root] pos{1}; mode[root,session] sub{on,off} pos{0} needs-session; reject[reason,root] pos{1}; serve[addr,root] pos{0}; setup[adopt,greenfield,root,supplement,verify] pos{0} --argv; status[root,session] pos{0}; turn declare[changes,root,session,units] pos{0} needs-session; turn diff[root,session] pos{0} needs-session; turn end[root,session] pos{0} needs-session; turn impact[add,root,session] pos{0} needs-session; turn show[root,session] pos{0}; turn start[request,root,seeds,session] pos{0} needs-session; turn verify[root,session] pos{0} --argv needs-session; exit error=1,needhuman=2,ok=0,usage=64`, purpose INFERRED: is the command surface humans, skills and the screen call
- IF-0003 screen HTTP (EXTRACTED), 2 anchor(s), signature `GET /; POST /act; act do={align,approve,confirm,confirm-reject,couple,realign,reject} form{do,from,id,key,kind,reason,t,to,value}`, purpose INFERRED: is what the screen page posts to decide
- IF-0004 confirm item key (EXTRACTED), 3 anchor(s), signature `ID.{purpose,responsibility} or ID.{consumes,depends_on,provides}.ID`, purpose INFERRED: names one guess in the confirm queue
- IF-0005 change item key (EXTRACTED), 3 anchor(s), signature `{contract-changed,new-relation,removed-relation,unit-added,unit-removed}:ID or :ID.{consumes,depends_on,provides}.ID`, purpose INFERRED: names one topology change so a human can approve or reject it
- MOD-0001 abbd CLI (EXTRACTED), 6 anchor(s), purpose INFERRED: lets the human and the agent drive ABBD from a terminal and from Claude Code hooks
- MOD-0002 PreToolUse guard (EXTRACTED), 2 anchor(s), purpose INFERRED: stops an agent in an active session from doing a human's step
- MOD-0003 screen server (EXTRACTED), 2 anchor(s), purpose INFERRED: gives the human a browser screen for every decision
- MOD-0004 internal/bootstrap (EXTRACTED), 3 anchor(s), purpose INFERRED: lays the supplement and adopted documents over the script result and writes the setup report
- MOD-0005 internal/confirm (EXTRACTED), 2 anchor(s), purpose INFERRED: is the queue of agent guesses waiting for a human
- MOD-0006 internal/extract (EXTRACTED), 2 anchor(s), purpose INFERRED: is the trust boundary where extraction JSON becomes typed units
- MOD-0007 internal/gitx (EXTRACTED), 2 anchor(s), purpose INFERRED: reads the few git facts ABBD needs without touching the user's index
- MOD-0008 internal/identity (EXTRACTED), 2 anchor(s), purpose INFERRED: keeps design unit IDs stable across re-extraction
- MOD-0009 internal/model (EXTRACTED), 2 anchor(s), purpose INFERRED: is the shared vocabulary of design units and their evidence
- MOD-0010 internal/screen (EXTRACTED), 4 anchor(s), purpose INFERRED: shows the human what needs a decision without opening Markdown
- MOD-0011 internal/store (EXTRACTED), 3 anchor(s), purpose INFERRED: persists all ABBD state as reviewable files under .abbd/ and the git dir
- MOD-0012 internal/topology (EXTRACTED), 2 anchor(s), purpose INFERRED: compares the approved intended topology with the actual one
- MOD-0013 internal/turn (EXTRACTED), 2 anchor(s), purpose INFERRED: models one unit of agent work under abbd mode

## supplement

31 unit(s) matched the code.
Conflicts: none.
Gaps: none.

## Confirm queue

73 item(s) wait for a human. None of them blocks alignment.
- CFG-0001.purpose "publishes ABBD as a Claude Code plugin from this repository"
- CFG-0001.responsibility "fixes the marketplace and plugin names and the plugin source path"
- CFG-0002.purpose "records the trusted extraction and verify argv"
- CFG-0002.responsibility "fixes project.yaml, which mode on and turn verify run without asking"
- CFG-0003.purpose "registers ABBD's hooks with Claude Code"
- CFG-0003.responsibility "fixes which events and tool matchers call abbd hook"
- DATA-0001.purpose "is what every project extraction script and supplement must print"
- DATA-0001.responsibility "fixes the units and relations JSON; unknown fields are rejected"
- DATA-0002.purpose "is the actual topology as reviewable files"
- DATA-0002.responsibility "fixes the unit file frontmatter: anchors, facts with their evidence, relations"
- DATA-0003.purpose "lets tools and humans read the topology without the CLI"
- DATA-0003.responsibility "fixes the generated file names and graph.json, whose watermark tells CURRENT from STALE"
- DATA-0004.purpose "is the record of each human decision on a grade C change"
- DATA-0004.responsibility "fixes ADR frontmatter: subject, decision, question, reason and turn"
- DATA-0005.purpose "is the human-approved topology every gate compares against"
- DATA-0005.responsibility "fixes intended.yaml: revision, units with signatures and external consumers, relations"
- DATA-0006.purpose "is the evidence that a turn's verification passed on its tree"
- DATA-0006.responsibility "fixes the run record: command, tree, exit code, result and log path"
- DATA-0007.purpose "is the record of each turn"
- DATA-0007.responsibility "fixes CHG frontmatter: request, seeds, impact, intent delta, approvals, rejections, verification and report"
- DATA-0008.purpose "keeps each user request verbatim"
- DATA-0008.responsibility "fixes the requirement file a turn points to"
- FEAT-0001.consumes.CFG-0001 "INFERRED"
- FEAT-0001.consumes.CFG-0003 "INFERRED"
- FEAT-0001.consumes.DATA-0005 "INFERRED"
- FEAT-0001.consumes.DATA-0007 "INFERRED"
- FEAT-0001.depends_on.MOD-0002 "INFERRED"
- FEAT-0001.purpose "runs a pstack playbook inside a gated ABBD turn"
- FEAT-0001.responsibility "maps playbooks to turn shapes, sequences start, impact, declare, diff, verify and end, holds the PR until SYNCED or CLOSED, and briefs abbd-agent delegates"
- FEAT-0002.consumes.DATA-0001 "INFERRED"
- FEAT-0002.purpose "bootstraps ABBD on a repository without asking the human"
- FEAT-0002.responsibility "drives the pstack swarm, the extraction script, the supplement and abbd setup, and leaves alignment to the human"
- IF-0001.purpose "is how Claude Code hands ABBD each session event"
- IF-0001.responsibility "fixes the hook events, the JSON fields read on stdin and the decision shape written on stdout"
- IF-0002.purpose "is the command surface humans, skills and the screen call"
- IF-0002.responsibility "fixes the commands, their flags, positional counts, argv passthrough and exit codes 0, 1, 2 and 64"
- IF-0003.purpose "is what the screen page posts to decide"
- IF-0003.responsibility "fixes the routes and the act form: which decisions exist and which fields each reads"
- IF-0004.purpose "names one guess in the confirm queue"
- IF-0004.responsibility "fixes the ID.field and edge syntax a human types to confirm or reject a guess"
- IF-0005.purpose "names one topology change so a human can approve or reject it"
- IF-0005.responsibility "fixes the change kinds and the ID and edge syntax that the CLI, the screen, CHG and ADR files share"
- MOD-0001.purpose "lets the human and the agent drive ABBD from a terminal and from Claude Code hooks"
- MOD-0001.responsibility "parses argv against the command table, runs setup, align, confirm, approve, reject, couple, ingest, mode, status and the turn commands, maps outcomes to exit codes, and republishes .abbd/generated/ only when the inputs hash changes"
- MOD-0002.consumes.CFG-0003 "INFERRED"
- MOD-0002.consumes.IF-0002 "INFERRED"
- MOD-0002.depends_on.MOD-0011 "INFERRED"
- MOD-0002.purpose "stops an agent in an active session from doing a human's step"
- MOD-0002.responsibility "splits shell text into simple commands and denies human-only abbd commands, writes under .abbd/, session state, the registered extraction script, and requests to the running screen"
- MOD-0003.purpose "gives the human a browser screen for every decision"
- MOD-0003.responsibility "listens on loopback only, checks Host, Origin, Sec-Fetch-Site and the per-launch token, and turns each form into one CLI argv run in process"
- MOD-0004.purpose "lays the supplement and adopted documents over the script result and writes the setup report"
- MOD-0004.responsibility "matches document units to script units by shared anchors, refuses ties, keeps code facts from the script, and renders a byte-stable bootstrap report"
- MOD-0005.purpose "is the queue of agent guesses waiting for a human"
- MOD-0005.responsibility "lists pending purposes, responsibilities and relations design units first, names them, and applies confirm and reject"
- MOD-0006.purpose "is the trust boundary where extraction JSON becomes typed units"
- MOD-0006.responsibility "validates kinds, names, anchors, unique keys and contract-only signatures, and marks code facts EXTRACTED and stated intent INFERRED"
- MOD-0007.purpose "reads the few git facts ABBD needs without touching the user's index"
- MOD-0007.responsibility "snapshots the working tree as a tree object through a temporary index, diffs snapshots for renames and changed paths, and reports HEAD and the git dir"
- MOD-0008.depends_on.MOD-0004 "INFERRED"
- MOD-0008.purpose "keeps design unit IDs stable across re-extraction"
- MOD-0008.responsibility "matches units by kind and two shared anchors or an identical set, mints IDs from the high-water mark, follows renames, records merge candidates, and decides which facts re-extraction may overwrite"
- MOD-0009.purpose "is the shared vocabulary of design units and their evidence"
- MOD-0009.responsibility "defines Unit, Fact, Relation and Anchor, the origin, verification and confirmation axes, the kinds and their directories, and KIND-NNNN IDs"
- MOD-0010.purpose "shows the human what needs a decision without opening Markdown"
- MOD-0010.responsibility "builds a deterministic view of pending C changes, drift, trust badges, the confirm queue and an ID-stable map, and renders the live page and the watermarked generated files"
- MOD-0011.purpose "persists all ABBD state as reviewable files under .abbd/ and the git dir"
- MOD-0011.responsibility "is the only reader and writer of every .abbd file layout: no-op-if-equal writes, ID high-water minting, the intended revision, input hashes and session mode files"
- MOD-0012.purpose "compares the approved intended topology with the actual one"
- MOD-0012.responsibility "owns the five change kinds, the single effect rule (BLOCKED, REALIGN, PROMOTE), phase derivation, and the select, approve and realign folds"
- MOD-0013.depends_on.MOD-0007 "INFERRED"
- MOD-0013.purpose "models one unit of agent work under abbd mode"
- MOD-0013.responsibility "owns turn states, impact computation, grades A, B and C, approvals and rejections, the declared-versus-actual contrast, and the verification evidence rule"

## Blind spots

Couplings the script cannot see, such as a shared table or an execution order, may exist. Register one by confirming it from the queue, then have the script learn it.

## Next

- `abbd align --all`
- `abbd align --confirm IDS`
- `abbd align --exclude IDS`
