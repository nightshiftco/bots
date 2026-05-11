---
name: bug-investigation
description: Investigate a reported bug against the codebase and identify the root cause.
---

# Bug investigation

When the user describes a bug, error, regression, or unexpected behavior:

1. **Identify the symptom precisely.** Extract from the user's message:
   the failing operation, the observed result, and (when stated) the
   expected result, environment, and reproduction steps.

2. **Search the codebase.** Use the `mcp__github__*` tools against the
   `nightshiftco/nightshift` repository to:
   - Search code for the failing function names, error strings, and
     log messages mentioned in the report.
   - Look at recent commits and PRs in areas matching the symptom.
   - Read the relevant files end-to-end before forming a hypothesis;
     do not infer behavior from filenames or function signatures alone.

3. **Form a root-cause hypothesis.** State which code path produces the
   symptom and why. Cite specific files with `path/to/file.go:LINE`
   (single line or range). If you are uncertain between multiple
   candidates, say so and rank them.

4. **Reply with a structured report.** Use this shape:

   ```
   *Summary*: <one sentence>
   *Root cause*: <one paragraph, with file:line citations>
   *Suggested fix*: <bullet list of concrete edits>
   *Confidence*: high | medium | low (and why)
   ```

5. **Do not propose speculative fixes** unrelated to the reported
   symptom. If you cannot identify a probable root cause from the
   codebase, say so and list what additional information would be
   needed (logs, repro steps, version pinning, etc.).
