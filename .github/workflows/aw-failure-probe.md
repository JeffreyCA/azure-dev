---
name: AW Failure Probe
description: Fork-only probe that forces a chosen gh-aw failure category.
on:
  workflow_dispatch:
    inputs:
      scenario:
        description: Failure category to force
        type: choice
        options: [missing_data, missing_safe_outputs, report_incomplete, missing_tool]
permissions:
  contents: read
  copilot-requests: write
engine:
  id: copilot
  model: copilot/gpt-5.6-sol
  args: ["--effort", "low"]
checkout: false
strict: true
network:
  allowed: [defaults]
safe-outputs:
  group-reports: true
  report-failed-jobs: false
  report-failure-as-issue:
    - "!missing_data"
    - "!missing_safe_outputs"
    - "!report_incomplete"
    - "!inference_access_error"
    - "!ai_credits_rate_limit_error"
  report-incomplete:
    create-issue: false
timeout-minutes: 5
---

# AW failure probe

This is a controlled test of failure reporting. The scenario is `${{ github.event.inputs.scenario }}`. Do exactly one of the following and then stop:

- `missing_data`: call `missing_data` with data type `probe` and reason `Forced by failure probe`.
- `missing_tool`: call `missing_tool` with tool `probe_tool` and reason `Forced by failure probe`.
- `report_incomplete`: call `report_incomplete` with reason `Forced by failure probe`.
- `missing_safe_outputs`: do not call any safe output tool. Reply with the text `probe done` and stop.
