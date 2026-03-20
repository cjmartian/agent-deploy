0a. Study `ralph/specs/*` with up to 500 parallel Sonnet subagents to learn the application specifications.
0b. Study @IMPLEMENTATION_PLAN.md.
0c. For reference, the application source code is in `.github/*`.

1. Your task is to implement functionality per the specifications using parallel subagents. Follow @IMPLEMENTATION_PLAN.md and choose the most important item to address. Before making changes, search the codebase (don't assume not implemented) using Sonnet subagents. You may use up to 50 parallel Sonnet subagents for searches/reads and only 1 Sonnet subagent for build/tests. Use Opus subagents when complex reasoning is needed (debugging, architectural decisions).
2. After implementing functionality or resolving problems, run the tests for that unit of code that was improved. If functionality is missing then it's your job to add it as per the application specifications. Ultrathink.
3. When you discover issues, immediately update @IMPLEMENTATION_PLAN.md with your findings using a subagent. When resolved, update and remove the item.
4. When the tests pass, update @IMPLEMENTATION_PLAN.md, then `git add -A` then `git commit` with a message describing the changes. After the commit, `git push` `git push origin refs/notes/commits`.

99999. Important: When authoring documentation, capture the why — tests and implementation importance.
999999. Important: Single sources of truth, no migrations/adapters. If tests unrelated to your work fail, resolve them as part of the increment.
9999999. Use Git commit notes to extensively log the changes made during implementation for future reference. Reference previous commmit notes when trying to understand why something was done, fetch notes when needed `git fetch origin refs/notes/commits:refs/notes/commits`.
99999999. You may add extra logging if required to debug issues.
999999999. Keep @IMPLEMENTATION_PLAN.md current with learnings using a subagent — future work depends on this to avoid duplicating efforts. Update especially after finishing your turn.
9999999999. When you learn something new about how to run the application, update @.github/agents/ralph-wiggum.md using a subagent but keep it brief. For example if you run commands multiple times before learning the correct command then that file should be updated.
99999999999. For any bugs you notice, resolve them or document them in @IMPLEMENTATION_PLAN.md using a subagent even if it is unrelated to the current piece of work.
9999999999999. Use the skill-improver agent to help debug any test failures that are hard to understand. If you find a test that is flaky or has an unclear failure, create a ticket in @IMPLEMENTATION_PLAN.md to investigate it and assign it to the skill-improver agent.
99999999999999. Implement functionality completely. Placeholders and stubs waste efforts and time redoing the same work.
999999999999999. When @IMPLEMENTATION_PLAN.md becomes large periodically clean out the items that are completed from the file using a subagent.
9999999999999999. If you find inconsistencies in the specs/* then use an Opus 4.5 subagent with 'ultrathink' requested to update the specs.
99999999999999999. IMPORTANT: Keep @.github/agents/ralph-wiggum.md operational only — status updates and progress notes belong in `IMPLEMENTATION_PLAN.md`. A bloated @.github/agents/ralph-wiggum.md pollutes every future loop's context.
999999999999999999. IMPORTANT: once full implementation is complete, output `<promise>DONE</promise>`