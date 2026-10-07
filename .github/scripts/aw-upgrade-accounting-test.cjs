const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

module.exports = async (actionsDir, core, github, context) => {
  const { scanDailyAIC } = require(path.join(actionsDir, "daily_aic_scan.cjs"));
  const { scanCacheEntry } = require(path.join(actionsDir, "daily_aic_cache_helpers.cjs"));
  const { createAPIBudget } = require(path.join(actionsDir, "daily_aic_api_budget.cjs"));
  const guardrail = require(path.join(actionsDir, "check_daily_aic_workflow_guardrail.cjs"));
  const { renderDailyAICSummary } = guardrail;
  const { buildDailyAICExceededContext } = require(path.join(actionsDir, "handle_agent_failure.cjs"));
  const { formatAICCredits } = require(path.join(actionsDir, "daily_aic_workflow_helpers.cjs"));
  const { loadBillableJobs } = require(path.join(actionsDir, "daily_aic_component_coverage.cjs"));
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "aw-v0915-accounting-"));
  const now = Date.now();
  const timestamp = new Date(now - 60_000).toISOString();
  const cachePath = path.join(tempDir, "scan.jsonl");
  const runs = Array.from({ length: 6 }, (_, index) => ({
    id: index + 1,
    workflow_id: 7,
    run_attempt: 1,
    status: "completed",
    conclusion: index === 0 ? "success" : "cancelled",
    created_at: timestamp,
    updated_at: timestamp,
    html_url: `https://github.com/example/project/actions/runs/${index + 1}`,
  }));
  const response = (data) => ({
    status: 200,
    headers: { "x-ratelimit-remaining": "4000" },
    data,
  });
  let accountingAvailable = false;
  let resolvedIds = [];
  const fixture = {
    github: { rest: { actions: { getWorkflowRun: async () => response({ workflow_id: 7 }) } } },
    context: { repo: { owner: "example", repo: "project" }, runId: 99 },
    budget: createAPIBudget(),
    artifactClient: {},
    getRunAIC: async (_client, id) => {
      resolvedIds.push(id);
      if (id === 1) return 809.28112;
      if (!accountingAvailable) throw new Error("Missing accounting for executed agent component");
      return 7;
    },
    listPage: async () => ({
      response: response({ workflow_runs: runs }),
      sourceRunCount: runs.length,
      lookupMode: "workflow_id",
    }),
    token: "synthetic",
    fallbackAIC: 1000,
    cachePath,
    now,
  };
  const total = (scan) => scan.countedRuns.reduce((sum, run) => sum + run.aic, 0);
  const estimated = (scan) => scan.countedRuns
    .filter((run) => run.source === "estimated")
    .reduce((sum, run) => sum + run.aic, 0);

  try {
    const first = await scanDailyAIC(fixture);
    assert.ok(Math.abs(total(first) - 5809.28112) < 0.000001);
    assert.equal(estimated(first), 5000);
    assert.ok(total(first) >= 5000);
    const entries = fs.readFileSync(cachePath, "utf8").trim().split("\n").map(JSON.parse);
    assert.equal(entries.filter((entry) => entry.source === "estimated").length, 5);
    core.info("PASS: missing cancellation accounting retains conservative enforcement and estimate provenance");

    const summary = renderDailyAICSummary("Validation", "synthetic", 5000, first.countedRuns, null, {
      candidateRunsCount: 6,
      inspectedRunsCount: 6,
      truncatedByRateLimit: false,
    });
    assert.ok(summary.includes(`| Recorded AIC | ${formatAICCredits(809.28112)} |`));
    assert.ok(summary.includes(`| Estimated AIC (unresolved accounting) | ${formatAICCredits(5000)} |`));
    assert.ok(summary.includes("not measured consumption"));
    const report = buildDailyAICExceededContext(true, String(total(first)), "5000", "5000");
    assert.ok(report.includes(`**Recorded AIC:** \`${formatAICCredits(809.28112)}\``));
    assert.ok(report.includes(`**Estimated AIC (unresolved accounting):** \`${formatAICCredits(5000)}\``));
    assert.ok(report.includes("estimates are not measured consumption"));
    assert.ok(!report.includes("has already consumed"));
    await core.summary.addRaw(summary).addRaw("\n\n" + report).write();
    core.info("PASS: activation summary and failure report distinguish recorded from estimated credits");

    resolvedIds = [];
    const unresolved = await scanDailyAIC(fixture);
    assert.deepEqual(resolvedIds, [2, 3, 4, 5, 6]);
    assert.equal(unresolved.cacheHits, 1);
    assert.equal(estimated(unresolved), 5000);
    core.info("PASS: unresolved estimates are retried rather than reused as recorded cache hits");

    accountingAvailable = true;
    resolvedIds = [];
    const reconciled = await scanDailyAIC(fixture);
    assert.deepEqual(resolvedIds, [2, 3, 4, 5, 6]);
    assert.equal(estimated(reconciled), 0);
    assert.ok(Math.abs(total(reconciled) - 844.28112) < 0.000001);
    assert.ok(total(reconciled) < 5000);
    core.info("PASS: available accounting replaces cached estimates and restores budget headroom");

    const legacy = scanCacheEntry(runs[1], 1000, "example/project", 7, now);
    delete legacy.source;
    fs.writeFileSync(cachePath, JSON.stringify(legacy) + "\n");
    resolvedIds = [];
    const migrated = await scanDailyAIC(fixture);
    assert.ok(resolvedIds.includes(2));
    assert.equal(migrated.countedRuns.find((run) => run.id === 2).aic, 7);
    assert.equal(migrated.countedRuns.find((run) => run.id === 2).source, "recorded");
    core.info("PASS: legacy cache values without provenance are re-resolved");

    const staleZero = scanCacheEntry(runs[1], 0, "example/project", 7, now);
    staleZero.coverage_version = 1;
    fs.writeFileSync(cachePath, JSON.stringify(staleZero) + "\n");
    resolvedIds = [];
    const corrected = await scanDailyAIC(fixture);
    assert.ok(resolvedIds.includes(2));
    assert.equal(corrected.countedRuns.find((run) => run.id === 2).aic, 7);
    const correctedEntries = fs.readFileSync(cachePath, "utf8").trim().split("\n").map(JSON.parse);
    assert.equal(correctedEntries.find((entry) => entry.run_id === 2).coverage_version, 2);
    core.info("PASS: recorded zero-credit entries from the broken coverage version are recalculated");

    const source = await github.rest.actions.getWorkflowRun({
      ...context.repo,
      run_id: 37532807067,
    });
    const components = await loadBillableJobs(
      { github, budget: createAPIBudget() },
      context.repo.owner,
      context.repo.repo,
      source.data,
    );
    assert.ok(components.has("agent"), "A completed v0.91.2 Agent job must be included in billable coverage");
    assert.ok(components.has("detection"), "A completed v0.91.2 Detection job must be included in billable coverage");
    core.info("PASS: real compiler-generated job names are recognized as billable components");

    const artifactClient = await guardrail.getArtifactClient();
    const actualAIC = await guardrail.getRunAIC(
      artifactClient,
      source.data.id,
      process.env.VALIDATION_GITHUB_TOKEN,
      context.repo.owner,
      context.repo.repo,
      source.data,
      { github, budget: createAPIBudget() },
    );
    assert.ok(Math.abs(actualAIC - 30.08748) < 0.001, `Unexpected recorded usage: ${actualAIC}`);
    core.info(`PASS: actual completed run accounts for ${actualAIC} recorded credits rather than zero`);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
};
