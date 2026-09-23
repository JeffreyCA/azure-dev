---
name: AW Failure Suppression Test
run-name: "AW Failure Suppression Test (${{ github.event.inputs.scenario }})"
on:
  workflow_dispatch:
    inputs:
      scenario:
        description: Failure signal to exercise.
        required: true
        type: choice
        options:
          - missing-safe-output
          - missing-data
          - report-incomplete
permissions:
  contents: read
  copilot-requests: write
  issues: read
engine:
  id: copilot
  model: copilot/gpt-5.6-sol
  args: ["--effort", "low"]
strict: true
network:
  allowed: [defaults, github]
safe-outputs:
  report-failed-jobs: false
  report-failure-as-issue:
    - "!missing_data"
    - "!missing_safe_outputs"
    - "!report_incomplete"
  missing-data:
    create-issue: false
  report-incomplete:
    create-issue: false
  add-labels:
    allowed: [automation]
    max: 1
timeout-minutes: 5
---

# AW failure suppression test

Exercise the `${{ github.event.inputs.scenario }}` scenario exactly as follows:

- For `missing-safe-output`, reply with `Intentional missing safe output test.` without calling any tool.
- For `missing-data`, call `missing_data` once with data type `test fixture`, reason `Intentional missing data test`, context `Fork workflow validation`, and alternative `No action required`.
- For `report-incomplete`, call `report_incomplete` once with reason `Intentional incomplete test` and details `Fork workflow validation`.

Do not call `add_labels`, `noop`, or any other tool.
