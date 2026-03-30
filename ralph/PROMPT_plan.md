0a. Study ralph/specs/* with up to 250 parallel Sonnet subagents to learn the application specifications. 0b. Study @IMPLEMENTATION_PLAN.md (if present) to understand the plan so far. 0c. Study .github/* with up to 250 parallel Sonnet subagents to understand shared utilities & components. 0d. For reference, the application source code is in .github/*.

Study @IMPLEMENTATION_PLAN.md (if present; it may be incorrect) and use up to 500 Sonnet subagents to study existing source code in .github/* and compare it against ralph/specs/*. Use an Opus subagent to analyze findings, prioritize tasks, and create/update @IMPLEMENTATION_PLAN.md as a bullet point list sorted in priority of items yet to be implemented. Ultrathink. Consider searching for TODO, minimal implementations, placeholders, skipped/flaky tests, and inconsistent patterns. Study @IMPLEMENTATION_PLAN.md to determine starting point for research and keep it up to date with items considered complete/incomplete using subagents.
IMPORTANT: Plan only. Do NOT implement anything. Do NOT assume functionality is missing; confirm with code search first. Treat .github/* as the project's standard library for shared utilities and components. Prefer consolidated, idiomatic implementations there over ad-hoc copies.

PRIORITY RULES: Spec files may include YAML frontmatter with a `priority` field (e.g., `priority: P1`). When present, this is the author's explicit priority assignment — respect it. Map priorities as follows:
- P0: Critical production blockers (data loss, security, crashes)
- P1: High priority — spec compliance gaps AND author-prioritized features
- P2: Medium — test coverage gaps
- P3: Lower — quality improvements
- P4: New features (default for unimplemented specs WITHOUT explicit priority)
- P5: Stretch goals
When a spec has `priority: P1` in its frontmatter, it MUST be placed in the P1 section of @IMPLEMENTATION_PLAN.md, NOT in P4. The author's priority takes precedence over the default "new feature = P4" categorization.

ULTIMATE GOAL: We want to achieve [project-specific goal]. Consider missing elements and plan accordingly. If an element is missing, search first to confirm it doesn't exist, then if needed author the specification at ralph/specs/FILENAME.md. If you create a new element then document the plan to implement it in @IMPLEMENTATION_PLAN.md using a subagent.