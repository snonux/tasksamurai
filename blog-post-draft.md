# Lessons from Building Task Samurai with Agentic Coding

## Introduction
Task Samurai is a terminal-based interface for Taskwarrior built entirely through agentic coding with OpenAI Codex. The project's git history spans June 19–22, 2025, with 179 commits. This post outlines the development journey, common pitfalls, and the time savings compared to manual coding.

## Timeline Overview
- **June 19**: Initial setup with boilerplate Go program and tests. The UI framework Bubble Tea was introduced along with early table-based views.
- **June 20**: Intense iteration phase with over 120 commits. Features such as hotkeys, colorization, annotation support, undo functionality, and fireworks on quit were added. Numerous bug fixes and merges highlight a highly iterative process.
- **June 21**: Enhancements to search, theming, dynamic column sizing, and extensive hotkey documentation. Continued refinements in UI behavior and layout.
- **June 22**: Final touches including screenshot assets, logo tweaks, and module path updates.

Commit statistics show heavy activity on June 20 with 120 commits, followed by refinement on the 21st and final polish on the 22nd.

## Common Pitfalls in Agentic Coding
1. **Merge Floods** – Most commits were merges from feature branches like `codex/add-hotkeys-for-task-actions`. Frequent merges caused noise and occasional conflicts.
2. **Repeated Fixes** – Many commits fix earlier commits: e.g., "fix fireworks exit" or "fix cell selection colors". Iterations sometimes introduced new bugs before stabilizing.
3. **Hotkey Revisions** – Hotkey behavior changed multiple times (swap, rename, or add). Coordinating these changes with documentation was tricky.
4. **UI Alignment Issues** – Commits such as `fix row alignment issue` and `right-align urgency` illustrate repeated tuning of the table layout.
5. **Feature Overload** – Rapid addition of features (search, theme toggling, tag editing, recurrence) sometimes overlapped, causing rework.

## Iteration Patterns
- **Scaffolding First** – Early commits established a table-based UI and command wrappers before focusing on features.
- **Small PRs** – Many merges correspond to small tasks or bug fixes, allowing quick feedback.
- **Test Utilization** – The project contains unit tests for task manipulation, ensuring changes didn’t break core functionality.
- **Documentation Updates** – Several commits like `docs: expand README with hotkey help` show the importance of keeping docs in sync with fast-paced feature changes.

## Approximate Time Savings
Manual implementation of 179 commits over four days would require a developer to write code, fix bugs, and update docs. Assuming:
- **5 minutes** average per commit for Codex to generate code and the developer to review = **~15 hours** of active guidance.
- A manual implementation might take roughly **60–80 hours**, considering design, testing, and debugging.

Thus, agentic coding potentially saved around **45–65 hours** of manual effort, compressing weeks of work into days.

## Conclusion
Task Samurai showcases both the power and pitfalls of agentic coding. Automated agents enabled rapid feature growth but also led to cycles of fixes and merges. The key takeaways are to maintain short iterations, keep tests and docs up to date, and expect plenty of polish passes. Despite the churn, building a full-featured terminal UI in just a few days demonstrates significant time savings and the promise of agentic development.
